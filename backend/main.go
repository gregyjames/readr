package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"golang.org/x/net/html"
)

type RequestBody struct {
	URL string `json:"url"`
}

func main(){
	app := fiber.New()

	app.Use(cors.New(cors.Config{
        AllowOrigins: "http://localhost:8080", // Vue dev server
        AllowHeaders: "Origin, Content-Type, Accept",
    }))
	
	app.Get("/", func(c *fiber.Ctx) error {
		//return c.SendString("Hello, World!")
		return c.JSON(fiber.Map{"message": "Hello from Go!"})
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
`, title, body.URL, imagePath, body.URL)

		filename := fmt.Sprintf("articles/%d.md", time.Now().Unix())
		os.MkdirAll("articles", os.ModePerm)
		os.WriteFile(filename, []byte(markdown), 0644)

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
	var src string
	var crawler func(*html.Node)
	crawler = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" && strings.HasPrefix(attr.Val, "http") {
					src = attr.Val
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawler(c)
		}
	}
	crawler(n)
	return src
}

func downloadImage(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	name := filepath.Base(strings.Split(url, "?")[0])
	os.MkdirAll("images", os.ModePerm)

	out, err := os.Create("images/" + name)
	if err != nil {
		return ""
	}
	defer out.Close()

	io.Copy(out, resp.Body)
	return "/images/" + name
}
