package torrent

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	client *Client
}

func NewHandler(client *Client) *Handler {
	return &Handler{
		client: client,
	}
}

type Magnet struct {
	Magnet   string `json:"magnet"`
	InfoHash string `json:"infoHash"`
}

func (h *Handler) AddMagnet(c *gin.Context) {
	var magnet Magnet
	if err := c.ShouldBindJSON(&magnet); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	spec, err := h.client.ParseMagnet(magnet.Magnet)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid magnet"})
		return
	}

	t, err := h.client.AddTorrentSpec(spec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "add torrent failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"infoHash": t.InfoHash().HexString()})
}
