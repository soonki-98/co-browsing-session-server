package main

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"co-browsing-session-server/internal/app"
)

// gen-openapi는 서버를 실제로 띄우지 않고, in-process로 라우터에 /openapi.yaml 요청을 보내
// 응답을 docs/openapi.yaml 파일로 기록한다. 런타임 endpoint와 동일한 huma API 인스턴스를 거치므로
// 런타임 스펙과 빌드 타임 스펙 사이의 drift가 발생할 수 없다.
func main() {
	engine := app.New().Engine()

	request := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		log.Fatalf("gen-openapi: expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	outputPath := filepath.Join("docs", "openapi.yaml")
	if err := os.WriteFile(outputPath, recorder.Body.Bytes(), 0o644); err != nil {
		log.Fatalf("gen-openapi: write %s: %v", outputPath, err)
	}

	log.Printf("gen-openapi: wrote %s (%d bytes)", outputPath, recorder.Body.Len())
}
