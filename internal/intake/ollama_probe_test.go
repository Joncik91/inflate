package intake

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeOllamaHappy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[
			{"name":"gemma4:26b","details":{"family":"gemma4","parameter_size":"26B","quantization_level":"Q4_K_M"}},
			{"name":"qwen3.6:35b","details":{"family":"qwen35moe","parameter_size":"36B","quantization_level":"Q4_K_M"}}
		]}`))
	}))
	defer srv.Close()

	models, ok := ProbeOllama(srv.URL)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Name != "gemma4:26b" || models[0].ParameterSize != "26B" {
		t.Errorf("first model wrong: %+v", models[0])
	}
}

func TestProbeOllamaFiltersEmbeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[
			{"name":"gemma4:26b","details":{"family":"gemma4","parameter_size":"26B","quantization_level":"Q4_K_M"}},
			{"name":"nomic-embed-text:latest","details":{"family":"nomic-bert","parameter_size":"137M","quantization_level":"F16"}}
		]}`))
	}))
	defer srv.Close()

	models, ok := ProbeOllama(srv.URL)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 chat model after filtering, got %d", len(models))
	}
	if models[0].Family != "gemma4" {
		t.Errorf("kept wrong model: %+v", models[0])
	}
}

func TestProbeOllamaOnlyEmbeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[
			{"name":"nomic-embed-text:latest","details":{"family":"nomic-bert","parameter_size":"137M","quantization_level":"F16"}}
		]}`))
	}))
	defer srv.Close()

	if _, ok := ProbeOllama(srv.URL); ok {
		t.Error("expected ok=false when only embedding models are present")
	}
}

func TestProbeOllamaUnreachable(t *testing.T) {
	if _, ok := ProbeOllama("http://127.0.0.1:1"); ok {
		t.Error("expected ok=false for unreachable daemon")
	}
}

func TestProbeOllamaBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()

	if _, ok := ProbeOllama(srv.URL); ok {
		t.Error("expected ok=false for malformed response")
	}
}
