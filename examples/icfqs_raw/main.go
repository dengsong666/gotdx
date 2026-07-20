package main

import (
	"context"
	"log"

	"github.com/bensema/gotdx"
)

func main() {
	client := gotdx.NewICFQS()

	raw, err := client.ICFQSTopicListRaw(context.Background(), "", "", 1)
	if err != nil {
		log.Fatalln(err)
	}

	tables := gotdx.ICFQSFormatTables(raw)
	if len(tables) == 0 {
		log.Println("icfqs topic list returned no tables")
		return
	}

	log.Printf("icfqs topic list rows=%d", len(tables[0].Rows))
	for i, row := range tables[0].Rows[:min(5, len(tables[0].Rows))] {
		log.Printf("#%d %+v", i+1, row)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
