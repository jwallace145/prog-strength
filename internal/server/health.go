package server

import (
	"net/http"

	"github.com/jwallace145/progressive-overload-fitness-tracker/internal/httpresp"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	httpresp.OK(w, "service is healthy", nil)
}
