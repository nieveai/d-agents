package agents

import (
	"fmt"
	"sort"
	"time"

	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

type ShoppingNotificationAgent struct {
	Db *database.ShoppingDB
}

func NewShoppingNotificationAgent() (*ShoppingNotificationAgent, error) {
	db, err := database.NewShoppingDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get shopping db: %w", err)
	}
	return &ShoppingNotificationAgent{Db: db}, nil
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

		// Find the lowest price in the most recent period
		mostRecentPeriod := productList[len(productList)-1].Date
		var recentPrices []float64
		for _, p := range productList {
			if p.Date.Equal(mostRecentPeriod) {
				recentPrices = append(recentPrices, p.Price)
			}
		}
		lowestRecentPrice := recentPrices[0]
		for _, price := range recentPrices {
			if price < lowestRecentPrice {
				lowestRecentPrice = price
			}
		}

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

		lowestPreviousPrice := previousPrices[0]
		for _, price := range previousPrices {
			if price < lowestPreviousPrice {
				lowestPreviousPrice = price
			}
		}

		if lowestRecentPrice < lowestPreviousPrice {
			notifications = append(notifications, fmt.Sprintf("Price drop for %s: $%.2f (was $%.2f)", name, lowestRecentPrice, lowestPreviousPrice))
		}
	}

	if len(notifications) > 0 {
		workload.Payload = []byte(fmt.Sprintf("Price drop alerts:\n%s", notifications))
	} else {
		workload.Payload = []byte("No price drops detected.")
	}

	return nil
}
