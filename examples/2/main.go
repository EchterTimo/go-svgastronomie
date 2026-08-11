package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	sv "github.com/EchterTimo/go-svgastronomie"
)

func onDay(day sv.Day) {
	// Handle the new day here, e.g., print the dishes for the day.
	fmt.Println("Date:", day.Time.Format("2006-01-02"))
	for _, dish := range day.Dishes {
		fmt.Println("Dish:", dish.Name, "Price:", dish.Price)
	}
}

func scrapeAndSave(url string, fileName string) {
	// Example usage of the svgastronomie package
	restaurantURL := url
	fmt.Println("Scraping restaurant from URL:", restaurantURL)
	restaurant, err := sv.ScrapeRestaurant(restaurantURL, nil)
	if err != nil {
		println("Error scraping restaurant:", err.Error())
		return
	}
	fmt.Println("DONE")
	// write to json file
	var jsonData bytes.Buffer
	encoder := json.NewEncoder(&jsonData)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(restaurant); err != nil {
		println("Error marshalling restaurant to JSON:", err.Error())
		return
	}
	err = os.WriteFile(fileName, jsonData.Bytes(), 0644)
	if err != nil {
		println("Error writing JSON to file:", err.Error())
		return
	}
	fmt.Println("JSON data written to", fileName)
}

func main() {
	scrapeAndSave("https://sv-gastronomie.de/menu/Hermes,%20Hamburg/Mittagsmen%C3%BC%20Restaurant", "hermes_hamburg_restaurant.json")
	scrapeAndSave("https://sv-gastronomie.de/menu/Hermes,%20Hamburg/Mittagsmen%C3%BC%20Bistro", "hermes_hamburg_bistro.json")
	scrapeAndSave("https://sv-gastronomie.de/menu/apoBank,%20D%C3%BCsseldorf/Mittagsmen%C3%BC", "apobank_duesseldorf.json")
	scrapeAndSave("https://sv-gastronomie.de/menu/Landratsamt,%20M%C3%BCnchen/Mittagsmen%C3%BC", "landratsamt_muenchen.json")
}
