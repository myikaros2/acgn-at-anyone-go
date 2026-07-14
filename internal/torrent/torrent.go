package torrent

import (
	"acgn-at-anyone-go/config"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/maypok86/otter/v2"
	"github.com/maypok86/otter/v2/stats"
	"github.com/pion/webrtc/v4"
)

const GiB uint64 = 1 << 30

type Client struct {
	client *torrent.Client
	cache  *otter.Cache[string, *Torrent]
	config *config.TorrentConfig
}

type Torrent struct {
	torrent *torrent.Torrent
	closer  storage.ClientImplCloser
}

func NewClient(config *config.TorrentConfig) *Client {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = config.DataDir
	cfg.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir:   cfg.DataDir,
		PieceCompletion: storage.NewMapPieceCompletion(),
		TorrentDirMaker: func(baseDir string, info *metainfo.Info, infoHash metainfo.Hash) string {
			return filepath.Join(baseDir, infoHash.HexString())
		},
	})
	cfg.ICEServerList = []webrtc.ICEServer{
		{
			URLs: config.ICEServers,
		},
	}
	cfg.DisableUTP = true
	cfg.Seed = true

	client, err := torrent.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	cl := &Client{
		client: client,
		config: config,
	}

	counter := stats.NewCounter()
	cache := otter.Must(&otter.Options[string, *Torrent]{
		MaximumWeight: config.MaxDisk * GiB,
		Weigher: func(infoHash string, t *Torrent) uint32 {
			if t != nil && t.torrent != nil && t.torrent.Info() != nil {
				return uint32(t.torrent.Length())
			}
			return uint32(GiB)
		},
		StatsRecorder: counter,
		OnDeletion: func(e otter.DeletionEvent[string, *Torrent]) {
			t := e.Value
			log.Printf(
				"deletionEvent remove torrent: %s\nCause:%s\n",
				t.torrent.InfoHash().HexString(),
				e.Cause)
			cl.RemoveTorrentAndFiles(t)
		},
	})

	cl.cache = cache

	cl.cleanupTimer(config.CleanInterval)

	return cl
}

func (cl *Client) AddTorrentSpec(spec *torrent.TorrentSpec) (*torrent.Torrent, error) {
	spec.Trackers = [][]string{cl.config.Trackers}
	storageCloser := storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir:   cl.config.DataDir,
		PieceCompletion: storage.NewMapPieceCompletion(),
		TorrentDirMaker: func(baseDir string, info *metainfo.Info, infoHash metainfo.Hash) string {
			return filepath.Join(baseDir, infoHash.HexString())
		},
	})
	spec.Storage = storageCloser
	t, isNew, err := cl.client.AddTorrentSpec(spec)
	if err != nil {
		return nil, err
	}

	if !isNew {
		cl.RefreshCache(t)
		return t, nil
	}

	go cl.DownloadAndCache(&Torrent{
		torrent: t,
		closer:  storageCloser,
	})

	return t, nil
}

func (cl *Client) ParseMagnet(magnet string) (*torrent.TorrentSpec, error) {
	spec, err := torrent.TorrentSpecFromMagnetUri(magnet)
	return spec, err
}

func (cl *Client) DownloadAndCache(t *Torrent) {
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	select {
	case <-t.torrent.GotInfo():
		if t.torrent.Length() > 4<<30 {
			t.torrent.Drop()
			return
		}
		t.torrent.DownloadAll()
		go func() {
			start := time.Now()
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-t.torrent.Closed():
					return

				case <-ticker.C:
					completed := t.torrent.BytesCompleted()
					length := t.torrent.Length()
					infoHash := t.torrent.InfoHash().HexString()
					if length > 0 && completed >= length {
						log.Printf("download completed: %s, times: %s", infoHash, time.Since(start))
						return
					}
					log.Printf("%s: download: %d, length: %d, peers: %d", infoHash, completed, length, t.torrent.Stats().ActivePeers)
				}
			}
		}()
		cl.AddCache(t)

	case <-timer.C:
		t.torrent.Drop()
	}
}

func (cl *Client) AddCache(t *Torrent) {
	log.Printf("add cache: %s", t.torrent.InfoHash().HexString())
	cl.cache.Set(t.torrent.InfoHash().HexString(), t)
}

func (cl *Client) RefreshCache(t *torrent.Torrent) {
	log.Printf("refresh cache: %s", t.InfoHash().HexString())
	cl.cache.GetIfPresent(t.InfoHash().HexString())
}

func (cl *Client) MaxCache() uint64 {
	return cl.cache.GetMaximum()
}

func (cl *Client) UsedCache() uint64 {
	return cl.cache.WeightedSize()
}

func (cl *Client) FreeCache() uint64 {
	return cl.cache.GetMaximum() - cl.cache.WeightedSize()
}

func (cl *Client) RemoveTorrentAndFiles(t *Torrent) {
	infoHash := t.torrent.InfoHash().HexString()
	t.torrent.Drop()
	<-t.torrent.Closed()
	if t.closer != nil {
		err := t.closer.Close()
		if err != nil {
			log.Printf("failed close storage: %v", err)
		}
		log.Println("closed storage: ", infoHash)
	}
	err := os.RemoveAll(filepath.Join(cl.config.DataDir, infoHash))
	if err != nil {
		log.Printf("failed remove torrent dir: %v", err)
		return
	}
	log.Printf("remove torrent dir: %s", infoHash)
}

func (cl *Client) cleanup() {
	torrents := cl.client.Torrents()
	for _, t := range torrents {
		_, b := cl.cache.GetEntryQuietly(t.InfoHash().HexString())
		if !b {
			cl.RemoveTorrentAndFiles(&Torrent{
				torrent: t,
			})
			log.Println("cleanup torrent: ", t.InfoHash().HexString())
		}
	}

	entries, err := os.ReadDir(cl.config.DataDir)
	if err != nil {
		log.Printf("failed read dir: %v", err)
	}
	for _, entry := range entries {
		infoHash := entry.Name()
		_, b := cl.cache.GetEntryQuietly(infoHash)
		if !b {
			err := os.RemoveAll(filepath.Join(cl.config.DataDir, infoHash))
			if err != nil {
				log.Printf("failed cleanup torrent dir: %v", err)
				return
			}
			log.Printf("cleanup torrent dir: %s", infoHash)
		}
	}
}

func (cl *Client) cleanupTimer(interval time.Duration) {
	timer := time.NewTicker(interval)
	go func() {
		for range timer.C {
			cl.cleanup()
			log.Println("cleanup")
		}
	}()
}
