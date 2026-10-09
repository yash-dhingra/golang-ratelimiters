package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	// "time"
)

func main() {
	fmt.Println("Hello World")
	var wg sync.WaitGroup
	for i := 0; i < 150; i++ {
		// time.Sleep(10 * time.Millisecond)
		wg.Add(1)
		go func() {

			resp, err := http.Get("http://localhost:8080/SlidingWindowLog/40")
			if err != nil {
				fmt.Printf("Error Occured: %s\n", err.Error())
			} else {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("Successful Execution= %s: %s\n", resp.Status, string(body))
				resp.Body.Close()
			}
			wg.Done()

		}()
	}
	wg.Wait()
}
