package models

type Wrestler struct {
	ID	       		int    	`json:"id"`
	WeightClass 	string	`json:"weight_class"`
	Name       		string 	`json:"name"`
	Year			string 	`json:"year"`
	Team       		string 	`json:"team"`
	Conference 		string 	`json:"conference"`
	WinPercentage 	string	`json:"win_percentage"`
	NCAAFinish		string 	`json:"ncaa_finish"`
}
