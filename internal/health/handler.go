package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	Health *Health
}

func NewHandler(health *Health) *Handler {
	return &Handler{
		Health: health,
	}
}

func (h *Handler) Ready(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}
