package models

type Wrestler struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Team       string `json:"team"`
	Conference string `json:"conference"`
	Wins       int    `json:"wins"`
	Losses     int    `json:"losses"`
	Weight     int    `json:"weight_class"`
}
