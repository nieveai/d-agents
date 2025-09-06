package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/chromedp"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Please provide a URL as a command-line argument.")
	}
	url := os.Args[1]

	// create context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// run task list
	var res string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Evaluate(`document.querySelectorAll('head, script, style, link, class, href').forEach(el => el.remove());`, nil),
		chromedp.OuterHTML("html", &res),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(res)
}
