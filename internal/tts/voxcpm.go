package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultURL = "http://127.0.0.1:8808"

// HealthStatus is the VoxCPM sidecar reachability probe result.
type HealthStatus struct {
	Reachable bool
	Ready     bool
	Loading   bool
}

// SpeakRequest is sent to the VoxCPM sidecar.
type SpeakRequest struct {
	Text               string  `json:"text"`
	Control            string  `json:"control,omitempty"`
	CFGValue           float64 `json:"cfg_value,omitempty"`
	InferenceTimesteps int     `json:"inference_timesteps,omitempty"`
	ReferenceWavPath   string  `json:"reference_wav_path,omitempty"`
	PromptWavPath      string  `json:"prompt_wav_path,omitempty"`
	PromptText         string  `json:"prompt_text,omitempty"`
	Normalize          bool    `json:"normalize,omitempty"`
}

// BookRequest generates one continuous voice track, split per sentence server-side.
type BookRequest struct {
	Sentences          []string `json:"sentences"`
	Control            string   `json:"control,omitempty"`
	CFGValue           float64  `json:"cfg_value,omitempty"`
	InferenceTimesteps int      `json:"inference_timesteps,omitempty"`
	Normalize          bool     `json:"normalize,omitempty"`
}

// Client calls a local VoxCPM HTTP sidecar.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	url := strings.TrimSpace(baseURL)
	if url == "" {
		url = DefaultURL
	}
	return &Client{
		BaseURL: strings.TrimRight(url, "/"),
		HTTP:    &http.Client{Timeout: 180 * time.Second},
	}
}

func (c *Client) healthHTTP() *http.Client {
	if c.HTTP != nil && c.HTTP.Transport != nil {
		return &http.Client{Timeout: 5 * time.Second, Transport: c.HTTP.Transport}
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (c *Client) Health(ctx context.Context) (bool, error) {
	st := c.HealthStatus(ctx)
	if !st.Reachable {
		return false, fmt.Errorf("voxcpm unreachable")
	}
	return st.Ready, nil
}

func (c *Client) HealthStatus(ctx context.Context) HealthStatus {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return HealthStatus{}
	}
	res, err := c.healthHTTP().Do(req)
	if err != nil {
		return HealthStatus{}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		return HealthStatus{Reachable: res != nil}
	}
	var out struct {
		OK      bool `json:"ok"`
		Loading bool `json:"loading"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return HealthStatus{Reachable: true}
	}
	loading := out.Loading || !out.OK
	return HealthStatus{
		Reachable: true,
		Ready:     out.OK,
		Loading:   loading,
	}
}

func (c *Client) Speak(ctx context.Context, speak SpeakRequest) ([]byte, error) {
	raw, err := json.Marshal(speak)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/tts", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("tts: %s", msg)
	}
	return body, nil
}

func (c *Client) SpeakBook(ctx context.Context, speak BookRequest) ([][]byte, error) {
	raw, err := json.Marshal(speak)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: 2 * time.Hour}
	if c.HTTP != nil && c.HTTP.Transport != nil {
		httpClient.Transport = c.HTTP.Transport
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/tts/book", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("tts book: %s", msg)
	}
	var out struct {
		Segments []string `json:"segments"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if len(out.Segments) != len(speak.Sentences) {
		return nil, fmt.Errorf("tts book: got %d segments, want %d", len(out.Segments), len(speak.Sentences))
	}
	segments := make([][]byte, len(out.Segments))
	for i, b64 := range out.Segments {
		segments[i], err = base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("tts book segment %d: %w", i, err)
		}
	}
	return segments, nil
}
