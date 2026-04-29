package proxy

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mostlygeek/llama-swap/proxy/config"
	"github.com/mostlygeek/llama-swap/proxy/gpu"
)

// addSettingsApiHandlers registers the /api/gpus and /api/config endpoints
// used by the UI Settings page. They are protected with the same API-key
// middleware as the rest of /api/.
func addSettingsApiHandlers(pm *ProxyManager) {
	g := pm.ginEngine.Group("/api", pm.apiKeyAuth())
	{
		g.GET("/gpus", pm.apiListGPUs)
		g.POST("/gpus/rescan", pm.apiRescanGPUs)
		g.PUT("/gpus/:id", pm.apiSetGPUEnabled)

		g.GET("/config/models", pm.apiListModels)
		g.POST("/config/models", pm.apiSaveModel)
		g.PUT("/config/models/:id", pm.apiSaveModelByID)
		g.DELETE("/config/models/:id", pm.apiDeleteModel)
	}
}

// gpuView is the JSON shape returned to the UI: detected device fields plus
// the user's enable/disable overlay from config.GPUs.
type gpuView struct {
	gpu.GPU
	// Enabled overrides the embedded GPU.Enabled to honor the config overlay.
	Enabled bool `json:"enabled"`
}

func (pm *ProxyManager) currentGPUView() []gpuView {
	pm.Lock()
	cached := pm.detectedGPUs
	overlay := pm.config.GPUs
	pm.Unlock()

	out := make([]gpuView, 0, len(cached))
	for _, g := range cached {
		v := gpuView{GPU: g, Enabled: true}
		if cfg, ok := overlay[g.ID]; ok {
			v.Enabled = cfg.Enabled
		}
		out = append(out, v)
	}
	return out
}

func (pm *ProxyManager) apiListGPUs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"gpus": pm.currentGPUView()})
}

func (pm *ProxyManager) apiRescanGPUs(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	found := gpu.Detect(ctx)

	pm.Lock()
	pm.detectedGPUs = found
	pm.Unlock()

	c.JSON(http.StatusOK, gin.H{"gpus": pm.currentGPUView()})
}

func (pm *ProxyManager) apiSetGPUEnabled(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if pm.configPath == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config path not set; settings API unavailable"})
		return
	}
	if err := config.SetGPUEnabled(pm.configPath, id, body.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// modelView is the JSON shape returned to the UI for the model list. It
// flattens the most useful fields from ModelConfig and includes the model ID.
type modelView struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name,omitempty"`
	Description      string                 `json:"description,omitempty"`
	Cmd              string                 `json:"cmd"`
	ConcurrencyLimit int                    `json:"concurrencyLimit"`
	AutoScale        config.AutoScaleConfig `json:"autoScale"`
	Aliases          []string               `json:"aliases,omitempty"`
}

func (pm *ProxyManager) apiListModels(c *gin.Context) {
	pm.Lock()
	models := pm.config.Models
	pm.Unlock()

	out := make([]modelView, 0, len(models))
	for id, m := range models {
		out = append(out, modelView{
			ID:               id,
			Name:             m.Name,
			Description:      m.Description,
			Cmd:              m.Cmd,
			ConcurrencyLimit: m.ConcurrencyLimit,
			AutoScale:        m.AutoScale,
			Aliases:          m.Aliases,
		})
	}
	c.JSON(http.StatusOK, gin.H{"models": out})
}

// saveModelRequest is the JSON body accepted by POST/PUT /api/config/models.
type saveModelRequest struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Cmd              string                 `json:"cmd"`
	ConcurrencyLimit int                    `json:"concurrencyLimit"`
	AutoScale        config.AutoScaleConfig `json:"autoScale"`
	Aliases          []string               `json:"aliases"`
}

func (pm *ProxyManager) saveModel(c *gin.Context, id string) {
	if pm.configPath == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config path not set; settings API unavailable"})
		return
	}
	var body saveModelRequest
	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id == "" {
		id = body.ID
	}
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing model id"})
		return
	}
	if body.Cmd == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing cmd"})
		return
	}
	mc := config.ModelConfig{
		Cmd:              body.Cmd,
		Name:             body.Name,
		Description:      body.Description,
		ConcurrencyLimit: body.ConcurrencyLimit,
		AutoScale:        body.AutoScale,
		Aliases:          body.Aliases,
	}
	if err := config.SetModel(pm.configPath, id, mc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "id": id})
}

func (pm *ProxyManager) apiSaveModel(c *gin.Context)     { pm.saveModel(c, "") }
func (pm *ProxyManager) apiSaveModelByID(c *gin.Context) { pm.saveModel(c, c.Param("id")) }

func (pm *ProxyManager) apiDeleteModel(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	if pm.configPath == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config path not set; settings API unavailable"})
		return
	}
	if err := config.DeleteModel(pm.configPath, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
