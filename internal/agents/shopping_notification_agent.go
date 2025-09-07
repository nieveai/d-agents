package agents

import (
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"sort"
	"strings"
	"time"

	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

type ShoppingNotificationAgent struct {
	Db *database.ShoppingDB
}

type SmtpConfig struct {
	From     string `json:"from"`
	Password string `json:"password"`
	To       string `json:"to"`
	SmtpHost string `json:"smtp_host"`
	SmtpPort string `json:"smtp_port"`
}

func NewShoppingNotificationAgent() (*ShoppingNotificationAgent, error) {
	db, err := database.NewShoppingDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get shopping db: %w", err)
	}
	return &ShoppingNotificationAgent{Db: db}, nil
}

func (a *ShoppingNotificationAgent) sendEmail(body string, config SmtpConfig) error {
	msg := []byte("To: " + config.To + "\r\n" +
		"Subject: Nieve AI Alert!\r\n" +
		"\r\n" +
		body + "\r\n")

	auth := smtp.PlainAuth("", config.From, config.Password, config.SmtpHost)
	err := smtp.SendMail(config.SmtpHost+":"+config.SmtpPort, auth, config.From, []string{config.To}, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}

func (a *ShoppingNotificationAgent) DoWork(workload *pb.Workload, genAIClient m.GenAIClient) error {
	products, err := a.Db.GetAllProducts()
	if err != nil {
		return fmt.Errorf("failed to get products: %w", err)
	}

	// Group products by name
	productsByName := make(map[string][]*database.Product)
	for _, p := range products {
		productsByName[p.Name] = append(productsByName[p.Name], p)
	}

	var notifications []string
	for name, productList := range productsByName {
		// Sort products by date
		sort.Slice(productList, func(i, j int) bool {
			return productList[i].Date.Before(productList[j].Date)
		})

		if len(productList) < 2 {
			continue
		}

		// Find the product with the lowest price in the most recent period
		mostRecentPeriod := productList[len(productList)-1].Date
		var recentProducts []*database.Product
		for _, p := range productList {
			if p.Date.Equal(mostRecentPeriod) {
				recentProducts = append(recentProducts, p)
			}
		}
		if len(recentProducts) == 0 {
			continue
		}
		lowestRecentProduct := recentProducts[0]
		for _, p := range recentProducts {
			if p.Price < lowestRecentProduct.Price {
				lowestRecentProduct = p
			}
		}
		lowestRecentPrice := lowestRecentProduct.Price

		// Find the lowest price in the previous period
		previousPeriod := time.Time{}
		for i := len(productList) - 1; i >= 0; i-- {
			if productList[i].Date.Before(mostRecentPeriod) {
				previousPeriod = productList[i].Date
				break
			}
		}

		if previousPeriod.IsZero() {
			continue
		}

		var previousPrices []float64
		for _, p := range productList {
			if p.Date.Equal(previousPeriod) {
				previousPrices = append(previousPrices, p.Price)
			}
		}
		if len(previousPrices) == 0 {
			continue
		}
		lowestPreviousPrice := previousPrices[0]
		for _, price := range previousPrices {
			if price < lowestPreviousPrice {
				lowestPreviousPrice = price
			}
		}

		if lowestRecentPrice < lowestPreviousPrice {
			url := "N/A"
			if lowestRecentProduct.URL.Valid {
				url = lowestRecentProduct.URL.String
			}
			notifications = append(notifications, fmt.Sprintf("Price drop for %s: $%.2f (was $%.2f). URL: %s", name, lowestRecentPrice, lowestPreviousPrice, url))
		}
	}

	if len(notifications) > 0 {
		message := strings.Join(notifications, "\n")
		fullMessage := fmt.Sprintf("Nieve AI alerts:\n%s", message)
		workload.Payload = []byte(fullMessage)

		if workload.Config != "" {
			var config SmtpConfig
			if err := json.Unmarshal([]byte(workload.Config), &config); err != nil {
				log.Printf("Failed to unmarshal SMTP config: %v", err)
			} else {
				if err := a.sendEmail(fullMessage, config); err != nil {
					log.Printf("Failed to send notification email: %v", err)
				}
			}
		} else {
			log.Println("SMTP config not found in workload, skipping email notification.")
		}
	} else {
		workload.Payload = []byte("No price drops detected.")
	}

	return nil
}

