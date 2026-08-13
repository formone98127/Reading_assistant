package server

import (
	"reading-assistant/internal/config"
	"reading-assistant/internal/library"
	"reading-assistant/internal/tts"
)

func applyRewriterTTS(cfg config.Config, r *library.Rewriter) {
	if r == nil {
		return
	}
	r.TTS = tts.NewClient(cfg.VoxCPMURL)
	r.TTSControl = cfg.VoxCPMControl
	r.TTSCfg = cfg.VoxCPMCfg
	if r.TTSCfg <= 0 {
		r.TTSCfg = 2.0
	}
	r.TTSTimesteps = cfg.VoxCPMTimesteps
	if r.TTSTimesteps <= 0 {
		r.TTSTimesteps = 10
	}
}
