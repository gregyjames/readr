package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"golang.org/x/net/html"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

type RequestBody struct {
	URL string `json:"url"`
}

type Article struct {
	gorm.Model
	ID uint `gorm:"unique"`
	Article  string
	Image string
	Title   string
}

func main(){
	app := fiber.New()

	app.Static("/articles", "/app/data/articles")
	app.Static("/images", "/app/data/images")
	
	app.Use(cors.New(cors.Config{
        AllowOrigins: "http://localhost:8080", // Vue dev server
        AllowHeaders: "Origin, Content-Type, Accept",
    }))
	
	fmt.Println(sql.Drivers())
	
	sqlDB, err := sql.Open("sqlite", "/app/data/data.sqlite")
	if err != nil {
		log.Fatal("sql.Open failed:", err)
	}

	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		log.Fatal("GORM failed:", err)
	}

	if err != nil {
		panic(fmt.Sprintf("failed to connect database: %s", err.Error()))
	}
	
	db.AutoMigrate(&Article{})
	
	app.Get("/", func(c *fiber.Ctx) error {
		//return c.SendString("Hello, World!")
		return c.JSON(fiber.Map{"message": "Hello from Go!"})
	})

	app.Get("/articles", func(c *fiber.Ctx) error {
		var articles []Article
		if err := db.Find(&articles).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to retrieve articles",
			})
		}
		return c.JSON(articles)
	})
	
	app.Post("/add", func(c *fiber.Ctx) error {
		var body RequestBody

		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return c.Status(400).SendString("Invalid JSON")
		}

		resp, err := http.Get(body.URL)
		if err != nil || resp.StatusCode != 200 {
			return c.Status(500).SendString("Failed to fetch the page")
		}
		defer resp.Body.Close()

		doc, err := html.Parse(resp.Body)
		if err != nil {
			return c.Status(500).SendString("Failed to parse HTML")
		}

		title := extractTitle(doc)
		imageURL := extractMainImage(doc)
		imagePath := ""

		if imageURL != "" {
			imagePath = downloadImage(imageURL)
		}

		// Create markdown content
		markdown := fmt.Sprintf(`---
title: "%s"
url: "%s"
image: "%s"
---

[Source](%s)
![Image](%s "Article Image")
`, title, body.URL, imagePath, body.URL, imagePath)

		filename_id := time.Now().Unix()
		filename := fmt.Sprintf("/app/data/articles/%d.md", filename_id)
		os.MkdirAll("/app/data/articles", os.ModePerm)
		os.WriteFile(filename, []byte(markdown), 0644)
		
		db.Create(&Article{
			ID: uint(filename_id),
			Title: title,
			Image: imagePath,
			Article: fmt.Sprintf("/articles/%d.md", filename_id),
		})

		return c.SendString("Article saved.")
	})
	app.Listen(":3000")
}

func extractTitle(n *html.Node) string {
	var title string
	var crawler func(*html.Node)
	crawler = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = n.FirstChild.Data
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawler(c)
		}
	}
	crawler(n)
	return strings.TrimSpace(title)
}

func extractMainImage(n *html.Node) string {
	// Check OpenGraph image
	var ogImage string
	var crawler func(*html.Node)
	crawler = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var prop, content string
			for _, attr := range n.Attr {
				if attr.Key == "property" && attr.Val == "og:image" {
					prop = attr.Val
				}
				if attr.Key == "content" {
					content = attr.Val
				}
			}
			if prop == "og:image" && content != "" {
				ogImage = content
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawler(c)
		}
	}
	crawler(n)
	if ogImage != "" {
		return ogImage
	}

	// Fall back to first <img>
	var firstImg string
	var findImg func(*html.Node)
	findImg = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" && attr.Val != "" {
					firstImg = attr.Val
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findImg(c)
		}
	}
	findImg(n)
	return firstImg
}


func downloadImage(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	name := filepath.Base(strings.Split(url, "?")[0])
	os.MkdirAll("/app/data/images", os.ModePerm)

	out, err := os.Create("/app/data/images/" + name)
	if err != nil {
		return ""
	}
	defer out.Close()

	io.Copy(out, resp.Body)
	return "http://localhost:3000/images/" + name
}
