package agents

import (
	"context"
	"regexp"

	"github.com/chromedp/chromedp"
)

// extractJSONArray finds and extracts the first JSON array from a string.
func extractJSONArray(s string) string {
	re := regexp.MustCompile(`(?s)\[.*\]`)
	return re.FindString(s)
}

// extractURL finds the first URL in a string.
func extractURL(s string) string {
	re := regexp.MustCompile(`https?://[^\s]+`)
	return re.FindString(s)
}

// getHTMLFromURL uses chromedp to get the HTML content of a URL.
func getHTMLFromURL(url string) (string, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var res string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Evaluate(`document.querySelectorAll('head, script, style, link').forEach(el => el.remove());`, nil),
		chromedp.OuterHTML("html", &res),
	)
	if err != nil {
		return "", err
	}
	return res, nil
}
