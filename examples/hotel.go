package main

import (
        . "github.com/aixiansheng/go-dispatch"
        . "fmt"
        . "time"
        . "net/http"
	. "encoding/json"
	. "context"
)

type Guest struct {
        Name string
        LengthOfStay int
}

var receptionDesk * Queue
var hotelGuests * Queue
var hotelIsOpen * Group
var hotelStaffOps * Queue

func receiveGuest(w ResponseWriter, r *Request) {
	var g Guest
	if err := NewDecoder(r.Body).Decode(&g); err != nil {
		panic(err)
	} else {
		receptionDesk.Async(func() {
			checkInGuest(&g)
		})
	}
}

func checkInGuest(g * Guest) {
        hotelIsOpen.Async(hotelGuests, func() {
                Printf("%v checked in for %v\n", g.Name, g.LengthOfStay)
                Sleep(Duration(g.LengthOfStay) * Second)
                checkOutGuest(g)
        })
}

func checkOutGuest(g * Guest) {
        Printf("%v checked out\n", g.Name)
}

func openForBusiness() {
        receptionDesk = QueueCreate(2) // There are two people at the reception desk
        hotelIsOpen = GroupCreate() // Group to track whether or not the hotel has guests
        hotelGuests = QueueCreateConcurrent() // A queue that concurrently handles hotel guest activities
        hotelStaffOps = QueueCreateConcurrent() // A concurrent queue representing hotel staff operations
}

func acceptGuestsForTime(t int) {
        m := NewServeMux()
        m.HandleFunc("/guest/new", receiveGuest)
        s := Server{ Addr: ":8000", Handler: m }
        go func() {
		s.ListenAndServe()
	}()

        hotelIsOpen.Async(hotelStaffOps, func() {
                Sleep(Duration(t) * Second)

		Println("Not receiving new guests anymore...")
                s.Shutdown(Background())
        })
}

func main() {
        openForBusiness()
        acceptGuestsForTime(10)

        // Wait for the hotel to close because it's no longer accepting guests and all guests have checked out.
        hotelIsOpen.Wait(FOREVER)

        Println("We're closed because the day is over and there are no guests left.")
}

