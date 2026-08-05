package main

// Location — место хранения. ParentID == nil означает место верхнего уровня.
type Location struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	ParentID *string `json:"parentId"`
}

// Item — вещь с привязкой к месту хранения и списком фотографий (data URL).
type Item struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	LocationID string   `json:"locationId"`
	Notes      string   `json:"notes"`
	Images     []string `json:"images"`
	UpdatedAt  string   `json:"updatedAt"`
}

// itemInput — тело запроса на создание/обновление вещи.
type itemInput struct {
	Name       string   `json:"name"`
	LocationID string   `json:"locationId"`
	Notes      string   `json:"notes"`
	Images     []string `json:"images"`
}

// locationInput — тело запроса на создание места.
type locationInput struct {
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	ParentID *string `json:"parentId"`
}
