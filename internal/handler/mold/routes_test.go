package mold

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesIfCCVMRegistersOnCCVM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ABLESTACK_NODE_ROLE", "ccvm")

	router := gin.New()
	if ok := RegisterRoutesIfCCVM(router.Group("/api/v1/mold")); !ok {
		t.Fatalf("RegisterRoutesIfCCVM = false, want true")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mold", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/mold status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRegisterRoutesIfCCVMDoesNotRegisterOnHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ABLESTACK_NODE_ROLE", "host")

	router := gin.New()
	if ok := RegisterRoutesIfCCVM(router.Group("/api/v1/mold")); ok {
		t.Fatalf("RegisterRoutesIfCCVM = true, want false")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mold", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/mold status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
