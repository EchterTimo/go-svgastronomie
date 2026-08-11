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

func main() {
	// Example usage of the svgastronomie package
	restaurantURL := "https://sv-gastronomie.de/menu/Hermes,%20Hamburg/Mittagsmen%C3%BC%20Restaurant"
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
	fileName := "restaurant.json"
	err = os.WriteFile(fileName, jsonData.Bytes(), 0644)
	if err != nil {
		println("Error writing JSON to file:", err.Error())
		return
	}
	fmt.Println("JSON data written to", fileName)
}
