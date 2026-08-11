package svgastronomie

import "time"

// Restaurant represents a restaurant available on sv-gastronomie.de.
type Restaurant struct {
	URL      string `json:"url"`
	Location string `json:"location"`
	MenuCard string `json:"menu_card"`
	Week     Week   `json:"week"`
}

// Week represents the menu for a specific week.
type Week struct {
	Days []Day `json:"days"`
}

// Day represents the menu for a specific day.
type Day struct {
	Time   time.Time `json:"time"` // Time is the current date in the format YYYY-MM-DD 00:00:00 UTC.
	Dishes []Dish    `json:"dishes"`
}

// Dish represents a single dish available on a specific day.
type Dish struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Price       float64  `json:"price"`
	Tags        []string `json:"tags"`
}
