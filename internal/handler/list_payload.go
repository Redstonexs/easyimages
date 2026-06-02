package handler

import (
	"easyimage/config"
	"easyimage/internal/service"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type imageListFile struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	WebPURL string `json:"webp_url,omitempty"`
	InfoURL string `json:"info_url"`
	DownURL string `json:"down_url"`
}

type imageListPayloadData struct {
	Title      string          `json:"title"`
	Date       string          `json:"date"`
	Search     string          `json:"search"`
	Limit      int             `json:"limit"`
	Total      int             `json:"total"`
	Files      []imageListFile `json:"files"`
	Today      string          `json:"today"`
	Yesterday  string          `json:"yesterday"`
	DateLinks  []gin.H         `json:"date_links"`
	Extensions []string        `json:"extensions"`
}

func imageListPayload(c *gin.Context, cfg *config.Config) (imageListPayloadData, error) {
	listDate := cfg.ListDate
	if listDate < 1 {
		listDate = 1
	}

	today := time.Now().Format("2006/01/02/")
	datePath := c.DefaultQuery("date", today)
	if !strings.HasSuffix(datePath, "/") {
		datePath += "/"
	}
	if !isSafeDatePath(datePath) {
		return imageListPayloadData{}, fmt.Errorf("invalid date parameter")
	}

	if datePath < time.Now().AddDate(0, 0, -listDate).Format("2006/01/02/") {
		datePath = today
	}

	limit := cfg.ListNumber
	if limit <= 0 {
		limit = 50
	}
	if num := c.Query("num"); num != "" {
		parsed, err := strconv.Atoi(num)
		if err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	if search != "" && !isSafeListSearch(search) {
		return imageListPayloadData{}, fmt.Errorf("invalid search parameter")
	}

	basePath := "." + cfg.Path + datePath
	if search != "" {
		basePath += "*." + search
	} else {
		basePath += "*.*"
	}

	fileNames, total := service.GetFileListLimited(basePath, cfg.ShowSort, limit)
	files := make([]imageListFile, 0, len(fileNames))
	for _, name := range fileNames {
		if !service.IsImageFile(name) {
			continue
		}
		relativePath := cfg.Path + datePath + name
		files = append(files, imageListFile{
			Name:    name,
			URL:     cfg.Domain + relativePath,
			WebPURL: service.GetWebPURL(relativePath, cfg),
			InfoURL: "/app/info?img=" + relativePath,
			DownURL: "/app/down?dw=" + relativePath,
		})
	}

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006/01/02/")
	dateLinks := make([]gin.H, 0, listDate)
	for i := 2; i <= listDate; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006/01/02/")
		dateLinks = append(dateLinks, gin.H{
			"date":  date,
			"label": fmt.Sprintf("%d天前", i),
		})
	}

	return imageListPayloadData{
		Title:      cfg.Title,
		Date:       datePath,
		Search:     search,
		Limit:      limit,
		Total:      total,
		Files:      files,
		Today:      today,
		Yesterday:  yesterday,
		DateLinks:  dateLinks,
		Extensions: []string{"jpg", "png", "gif", "webp"},
	}, nil
}

func isSafeListSearch(search string) bool {
	for _, ch := range search {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}

func isSafeDatePath(datePath string) bool {
	if len(datePath) != len("2006/01/02/") {
		return false
	}
	for i, ch := range datePath {
		switch i {
		case 4, 7, 10:
			if ch != '/' {
				return false
			}
		default:
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func setFrontendAPIHeaders(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	c.Header("Vary", "Cookie")
}
