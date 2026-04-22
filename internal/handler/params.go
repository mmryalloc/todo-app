package handler

import (
	"net/http"
	"strconv"
)

func pageLimitFromRequest(r *http.Request) (int, int) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	limit := 10
	if p, err := strconv.Atoi(pageStr); err == nil {
		page = p
	}
	if l, err := strconv.Atoi(limitStr); err == nil {
		limit = l
	}

	return page, limit
}
