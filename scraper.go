package svgastronomie

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"
)

var pricePattern = regexp.MustCompile(`[-+]?\d+(?:[.,]\d+)?`)

var errNoNumericPrice = errors.New("no numeric price found")

// Install checks if Playwright is installed and installs it if not.
func Install() {
	if _, err := playwright.Run(); err != nil {
		fmt.Println("Playwright is not installed. Installing...")
		if err := playwright.Install(&playwright.RunOptions{
			Browsers:         []string{"chromium"},
			OnlyInstallShell: true,
		}); err != nil {
			log.Fatalf("could not install playwright: %v", err)
		}
	}
}

// getDaysOfWeek returns the days of the week in German abbreviations.
func getDaysOfWeek() []string {
	return []string{"Mo.", "Di.", "Mi.", "Do.", "Fr."}
}

// getWeekStart returns the date (time.Time) of the Monday of the current week.
func getWeekStart(now time.Time) time.Time {
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
}

func resolveMenuURL(rawURL string) string {
	if rawURL == "" {
		return MenuURL
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	encodedRestaurant := encodeMenuSegment(rawURL)
	encodedMenu := encodeMenuSegment("Mittagsmenü")
	return fmt.Sprintf(MenuTemplateURL, encodedRestaurant, encodedMenu)
}

func encodeMenuSegment(value string) string {
	encoded := url.PathEscape(value)
	encoded = strings.ReplaceAll(encoded, "%2C", ",")
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	return encoded
}

// parseTitle splits the title into location and menu card name.
func parseTitle(title string) (string, string) {
	const minTitleParts = 2

	parts := strings.Split(title, "|")
	if len(parts) >= minTitleParts {
		location := strings.TrimSpace(parts[1])
		menuCard := strings.TrimSpace(parts[0])

		return location, menuCard
	}

	parts = strings.Split(title, " _ ")
	if len(parts) >= minTitleParts {
		location := strings.TrimSpace(parts[1])
		menuCard := strings.TrimSpace(parts[0])

		return location, menuCard
	}

	return "", ""
}

func parseLocationAndMenuFromURL(rawURL string) (string, string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}

	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "menu" {
		return "", ""
	}

	location, err := url.PathUnescape(pathParts[1])
	if err != nil {
		location = pathParts[1]
	}
	menuCard, err := url.PathUnescape(pathParts[2])
	if err != nil {
		menuCard = pathParts[2]
	}

	return location, menuCard
}

func parsePrice(raw string) (float64, error) {
	cleanPrice := strings.TrimSpace(raw)
	cleanPrice = strings.ReplaceAll(cleanPrice, "\u00a0", "")
	cleanPrice = strings.ReplaceAll(cleanPrice, "€", "")
	match := pricePattern.FindString(cleanPrice)
	if match == "" {
		return 0, fmt.Errorf("%w in %q", errNoNumericPrice, raw)
	}
	cleanPrice = strings.ReplaceAll(match, ",", ".")
	return strconv.ParseFloat(cleanPrice, 64)
}

func parseTagsFromElement(element playwright.Locator, dayName string) []string {
	rawTags, err := element.Locator("app-product-custom-tag img").EvaluateAll(`nodes => nodes
		.map(node => node.getAttribute("title"))
		.filter(Boolean)`)
	if err != nil {
		log.Printf("could not read tags for day %s: %v", dayName, err)
		return nil
	}

	tagsList, ok := rawTags.([]any)
	if !ok {
		return nil
	}

	tags := make([]string, 0, len(tagsList))
	for _, rawTag := range tagsList {
		tag, ok := rawTag.(string)
		if !ok {
			continue
		}
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	return tags
}

// ScrapeRestaurant scrapes the restaurant's page and returns a Restaurant object.
//
// The handleNewDay is an optional callback function that can be used to handle new days as they are scraped.
//
//nolint:funlen // Browser automation flow is sequential and clearer as one function.
func ScrapeRestaurant(
	url string,
	handleNewDay func(day Day),
) (*Restaurant, error) {
	Install()

	pw, err := playwright.Run() //nolint:varnamelen
	if err != nil {
		return nil, fmt.Errorf("could not start playwright: %w", err)
	}
	defer func() {
		if err := pw.Stop(); err != nil {
			log.Printf("could not stop playwright: %v", err)
		}
	}()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Args:     []string{"--disable-web-security"},
		Headless: new(true),
	})
	if err != nil {
		return nil, fmt.Errorf("could not launch browser: %w", err)
	}
	defer func() {
		if err := browser.Close(); err != nil {
			log.Printf("could not close browser: %v", err)
		}
	}()

	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Locale:     new("de-DE"),
		TimezoneId: new("Europe/Berlin"),
	})
	if err != nil {
		return nil, fmt.Errorf("could not create page: %w", err)
	}

	// Navigate to restaurant website.
	menuURL := resolveMenuURL(url)
	if _, err = page.Goto(menuURL); err != nil {
		return nil, fmt.Errorf("could not goto %s: %w", menuURL, err)
	}

	// get title and parse it into location and menu card name
	title, err := page.Title()
	if err != nil {
		return nil, fmt.Errorf("could not get title: %w", err)
	}
	location, menuCard := parseTitle(title)
	if location == "" || menuCard == "" {
		fallbackLocation, fallbackMenuCard := parseLocationAndMenuFromURL(menuURL)
		if location == "" {
			location = fallbackLocation
		}
		if menuCard == "" {
			menuCard = fallbackMenuCard
		}
	}

	cookiesReject := page.Locator("#cookiescript_reject")
	if err = cookiesReject.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err == nil {
		if err := page.Locator("#cookiescript_reject").Click(); err != nil {
			log.Printf("could not reject cookies: %v", err)
		}
	}

	weekStart := getWeekStart(time.Now())
	weekDays := make([]Day, 0, len(getDaysOfWeek()))

	for index, dayName := range getDaysOfWeek() {
		if err := page.Locator(fmt.Sprintf("a:has-text('%s')", dayName)).Click(); err != nil {
			return nil, fmt.Errorf("could not click day %s: %w", dayName, err)
		}

		if err := page.Locator("app-category").First().WaitFor(playwright.LocatorWaitForOptions{
			State: playwright.WaitForSelectorStateVisible,
		}); err != nil {
			return nil, fmt.Errorf("could not wait for dishes for day %s: %w", dayName, err)
		}

		elements, err := page.Locator("app-category:visible").All()
		if err != nil {
			return nil, fmt.Errorf("could not get elements for day %s: %w", dayName, err)
		}

		dishes := make([]Dish, 0, len(elements))
		for _, element := range elements {
			title, err := element.Locator("span.pre-wrap").TextContent()
			if err != nil {
				log.Printf("skipping dish without readable title for day %s: %v", dayName, err)
				continue
			}

			descriptionText, err := element.Locator("div.product-teaser").TextContent()
			if err != nil {
				descriptionText = ""
			}
			var description string
			if trimmed := strings.TrimSpace(descriptionText); trimmed != "" {
				description = trimmed
			}

			priceText, err := element.Locator("div.price").TextContent()
			if err != nil {
				priceText, err = element.Locator("div.price-column").TextContent()
			}
			if err != nil {
				log.Printf("skipping dish without readable price for day %s: %v", dayName, err)
				continue
			}
			price, err := parsePrice(priceText)
			if err != nil {
				log.Printf("skipping dish with unparseable price %q for day %s: %v", priceText, dayName, err)
				continue
			}

			tags := parseTagsFromElement(element, dayName)

			dishes = append(dishes, Dish{
				Name:        strings.TrimSpace(title),
				Description: description,
				Price:       price,
				Tags:        tags,
			})
		}

		day := Day{
			Time:   weekStart.AddDate(0, 0, index),
			Dishes: dishes,
		}
		if handleNewDay != nil {
			handleNewDay(day)
		}
		weekDays = append(weekDays, day)
	}

	return &Restaurant{
		URL:      menuURL,
		Week:     Week{Days: weekDays},
		Location: location,
		MenuCard: menuCard,
	}, nil
}
