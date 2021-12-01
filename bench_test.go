package main

import (
	"fmt"
	"os"
	"testing"
	"uk.ac.bris.cs/gameoflife/gol"
)

func BenchmarkGol(b *testing.B) {
	os.Stdout = nil
	for threads := 1; threads <= 16; threads++ {
		p := gol.Params{
			Turns:       1000,
			Threads:     threads,
			ImageWidth:  256,
			ImageHeight: 256,
		}
		testName := fmt.Sprintf("%dx%dx%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, p.Threads)
		b.Run(testName, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				events := make(chan gol.Event)
				go gol.Run(p, events, nil)
				for range events {
				}
			}
		})
	}
}
