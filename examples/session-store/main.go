package main

import (
	"context"
	"fmt"
	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/config"
	"github.com/nsigel/usc-cli/site"
	"sync"
)

type cookieStore struct{ cookies []auth.StoredCookie }

func (s *cookieStore) Load() ([]auth.StoredCookie, error)     { return s.cookies, nil }
func (s *cookieStore) Save(cookies []auth.StoredCookie) error { s.cookies = cookies; return nil }
func main() {
	path, err := config.SessionPath()
	if err != nil {
		panic(err)
	}
	var wg sync.WaitGroup
	for range 2 {
		cookies, err := auth.LoadStoredCookies(path)
		if err != nil {
			panic(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			store := &cookieStore{cookies: cookies}
			_, err := auth.Open(context.Background(), site.WebReg, auth.Options{SessionStore: store})
			if err != nil {
				panic(err)
			}
			fmt.Println("independent session opened and saved")
		}()
	}
	wg.Wait()
}
