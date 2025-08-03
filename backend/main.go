package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	markdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-shiori/go-readability"
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
	Article  string `json:"article"`
	Image string `json:"image"`
	Title   string `json:"title"`
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

	app.Get("/getarticles", func(c *fiber.Ctx) error {
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

	// Read HTML body into bytes (for readability)
	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).SendString("Failed to read HTML body")
	}

	parsedURL, err := url.Parse(body.URL)
	if err != nil {
		return c.Status(400).SendString("Invalid URL")
	}

	// Parse with readability
	article, err := readability.FromReader(bytes.NewReader(htmlBytes), parsedURL)
	if err != nil {
		return c.Status(500).SendString("Failed to parse readable content")
	}

	filenameID := time.Now().Unix()
	os.MkdirAll(fmt.Sprintf("/app/data/images/%d", filenameID), os.ModePerm)
	doc, _ := html.Parse(bytes.NewReader(htmlBytes))
	images := extractImageSources(doc)
	
	converter := markdown.NewConverter("", true, &markdown.Options{})
	
	markdownContent, err := converter.ConvertString(article.Content)

	for _, imgURL := range images {
		imgResp, err := http.Get(imgURL)
		if err != nil || imgResp.StatusCode != 200 {
			fmt.Println("Failed to download:", imgURL)
			continue
		}
		defer imgResp.Body.Close()

		// Extract filename
		parts := strings.Split(imgURL, "/")
		filename := parts[len(parts)-1]
		savePath := fmt.Sprintf("/app/data/images/%d/", filenameID) + filename

		// Save file
		out, _ := os.Create(savePath)
		io.Copy(out, imgResp.Body)
		out.Close()

		// Step 3: Replace image URLs in Markdown
		markdownContent = strings.ReplaceAll(markdownContent, imgURL, fmt.Sprintf("http://localhost:3000/images/%d/", filenameID)+filename)
	}
	
	if err != nil {
		return c.Status(500).SendString("Failed to convert HTML to markdown")
	}

	// Extract title & image
	title := article.Title
	imageURL := article.Image
	imagePath := ""

	if imageURL != "" {
		imagePath = downloadImage(imageURL, filenameID)
	}

	// Generate markdown with clean content
	filename := fmt.Sprintf("/app/data/articles/%d.md", filenameID)
	os.MkdirAll("/app/data/articles", os.ModePerm)

	markdown := fmt.Sprintf(`
[Source](%s)

![Cover Image](%s)

%s
`, body.URL, imagePath, markdownContent)

	err = os.WriteFile(filename, []byte(markdown), 0644)
	if err != nil {
		return c.Status(500).SendString("Failed to save markdown file")
	}

	// Save article entry in DB
	db.Create(&Article{
		Title:   title,
		Image:   imagePath,
		Article: fmt.Sprintf("/articles/%d.md", filenameID),
	})

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Article saved",
		"id":      filenameID,
	})
})

	app.Listen(":3000")
}

func downloadImage(url string, dirname int64) string {
	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	name := filepath.Base(strings.Split(url, "?")[0])
	directory := fmt.Sprintf("/app/data/images/%d/", dirname)
	os.MkdirAll(directory, os.ModePerm)

	out, err := os.Create(directory + name)
	if err != nil {
		return ""
	}
	defer out.Close()

	io.Copy(out, resp.Body)
	return fmt.Sprintf("http://localhost:3000/images/%d/", dirname) + name
}

func extractImageSources(n *html.Node) []string {
	var srcs []string
	var crawler func(*html.Node)
	crawler = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					srcs = append(srcs, attr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawler(c)
		}
	}
	crawler(n)
	return srcs
}