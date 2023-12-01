package main

import (
	"fmt"

	neosay "github.com/donuts-are-good/libneosay"
)

var say *neosay.Neosay

func startNeosay() {
	neosay, err := neosay.NewNeosay("config/neosay.json")
	if err != nil {
		fmt.Println("Error initializing Neosay:", err)
		return
	}

	message := "v" + BuildNumber + " loba launched"
	err = neosay.SendMessage("launch", message)
	if err != nil {
		fmt.Printf("Error sending message to %s: %v\n", "launch", err)
	}

	say = neosay

}
