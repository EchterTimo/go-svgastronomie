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
	pw, err := playwright.Run() //nolint:varnamelen
	if err != nil {
		fmt.Println("Playwright is not installed. Installing...")
		if err := playwright.Install(&playwright.RunOptions{
			Browsers:         []string{"chromium"},
			OnlyInstallShell: true,
		}); err != nil {
			log.Fatalf("could not install playwright: %v", err)
		}
		return
	}

	if err := pw.Stop(); err != nil {
		log.Printf("could not stop playwright in install check: %v", err)
	}
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

// rawDish holds unstructured dish data scraped from the website before parsing into a Dish object.
type rawDish struct {
	Name        string
	Description string
	Price       string
	Tags        []string
}

func parseRawDish(value any) (rawDish, bool) {
	dishMap, ok := value.(map[string]any)
	if !ok {
		return rawDish{}, false
	}

	rawName, _ := dishMap["name"].(string)
	rawDescription, _ := dishMap["description"].(string)
	rawPrice, _ := dishMap["price"].(string)

	tags := make([]string, 0)
	if rawTags, ok := dishMap["tags"].([]any); ok {
		tags = make([]string, 0, len(rawTags))
		for _, rawTag := range rawTags {
			tag, ok := rawTag.(string)
			if !ok {
				continue
			}
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	return rawDish{
		Name:        strings.TrimSpace(rawName),
		Description: strings.TrimSpace(rawDescription),
		Price:       strings.TrimSpace(rawPrice),
		Tags:        tags,
	}, true
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
		Args:     []string{"--disable-web-security", "--lang=de-DE"},
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

	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		ExtraHttpHeaders: map[string]string{
			"Accept-Language": "de-DE,de;q=0.9,en;q=0.8",
		},
		Locale:     new("de-DE"),
		TimezoneId: new("Europe/Berlin"),
	})
	if err != nil {
		return nil, fmt.Errorf("could not create browser context: %w", err)
	}
	defer func() {
		if err := context.Close(); err != nil {
			log.Printf("could not close browser context: %v", err)
		}
	}()

	page, err := context.NewPage()
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
	if err := page.Locator("a.mat-mdc-tab-link").First().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		return nil, fmt.Errorf("could not wait for day tabs to render: %w", err)
	}

	rawDayLabels, err := page.Evaluate(`() => {
		const tabs = Array.from(document.querySelectorAll("a.mat-mdc-tab-link"))
			.filter(tab => /\d{2}\.\d{2}\./.test((tab.textContent || "").trim()));
		return tabs
			.map(tab => (tab.textContent || "").trim())
			.filter(Boolean)
			.slice(0, 5);
	}`)
	if err != nil {
		return nil, fmt.Errorf("could not read top-level day tabs: %w", err)
	}

	daysOfWeek := make([]string, 0, 5) //nolint:mnd
	if dayLabelsAny, ok := rawDayLabels.([]any); ok {
		for _, rawDayLabel := range dayLabelsAny {
			dayLabel, ok := rawDayLabel.(string)
			if !ok {
				continue
			}
			dayLabel = strings.TrimSpace(dayLabel)
			if dayLabel == "" {
				continue
			}
			daysOfWeek = append(daysOfWeek, dayLabel)
		}
	}
	if dayLabels, ok := rawDayLabels.([]string); ok {
		for _, dayLabel := range dayLabels {
			dayLabel = strings.TrimSpace(dayLabel)
			if dayLabel == "" {
				continue
			}
			daysOfWeek = append(daysOfWeek, dayLabel)
		}
	}
	if len(daysOfWeek) == 0 {
		return nil, errors.New("top-level day tabs are empty")
	}

	weekDays := make([]Day, 0, len(daysOfWeek))

	for index, dayLabel := range daysOfWeek {
		rawDishes, err := page.Evaluate(`async ([tabIndex]) => {
			const tabs = Array.from(document.querySelectorAll("a.mat-mdc-tab-link"))
				.filter(tab => /\d{2}\.\d{2}\./.test((tab.textContent || "").trim()));
			const selectedTab = tabs[tabIndex];
			if (!selectedTab) {
				return [];
			}

			selectedTab.click();

			const normalize = (value) => (value || "").replace(/\s+/g, "").trim();
			const selectedDayText = (selectedTab.textContent || "").trim();

			const extractDishes = () => {
				const panelId = selectedTab.getAttribute("aria-controls");
				const panel = panelId ? document.getElementById(panelId) : null;
				if (!panel) {
					return [];
				}

				const menuCards = Array.from(panel.querySelectorAll("app-menu-card"));
				const targetCard = menuCards.find(card => {
					const activeDayText = card.querySelector("a.mat-mdc-tab-link.mdc-tab--active strong")?.textContent
						?? card.querySelector("a.mat-mdc-tab-link.mdc-tab--active")?.textContent
						?? "";
					const activeNormalized = normalize(activeDayText);
					const selectedNormalized = normalize(selectedDayText);
					return activeNormalized !== ""
						&& (selectedNormalized.includes(activeNormalized) || activeNormalized.includes(selectedNormalized));
				}) ?? menuCards[menuCards.length - 1] ?? panel;

				const categories = Array.from(targetCard.querySelectorAll("app-category"));
				return categories.map(node => {
					const name = node.querySelector("span.pre-wrap")?.textContent ?? "";
					const description = node.querySelector("div.product-teaser")?.textContent ?? "";
					const price = node.querySelector("div.price")?.textContent
						?? node.querySelector("div.price-column")?.textContent
						?? "";
					const tags = Array.from(node.querySelectorAll("app-product-custom-tag img"))
						.map(tagNode => tagNode.getAttribute("title"))
						.filter(Boolean);

					return { name, description, price, tags };
				});
			};

			for (let attempt = 0; attempt < 40; attempt += 1) {
				const dishes = extractDishes();
				const hasNamedDish = dishes.some(dish => (dish.name || "").trim() !== "");
				if (selectedTab.classList.contains("mdc-tab--active") && hasNamedDish) {
					return dishes;
				}
				await new Promise(resolve => setTimeout(resolve, 125));
			}

			return extractDishes();
		}`, []any{index})
		if err != nil {
			return nil, fmt.Errorf("could not load dishes for day %q: %w", dayLabel, err)
		}

		rawDishList, ok := rawDishes.([]any)
		if !ok {
			return nil, fmt.Errorf("unexpected dishes payload for day %q", dayLabel)
		}

		dishes := make([]Dish, 0, len(rawDishList))
		for _, raw := range rawDishList {
			rawDishData, ok := parseRawDish(raw)
			if !ok {
				continue
			}
			if rawDishData.Name == "" {
				continue
			}

			price, err := parsePrice(rawDishData.Price)
			if err != nil {
				log.Printf("skipping dish with unparseable price %q for day %q: %v", rawDishData.Price, dayLabel, err)
				continue
			}

			dishes = append(dishes, Dish{
				Name:        rawDishData.Name,
				Description: rawDishData.Description,
				Price:       price,
				Tags:        rawDishData.Tags,
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
