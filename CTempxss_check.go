package main

import (
	"fmt"
	"strings"
)

func main() {
	type TR struct{ Number, Description, Owner string }
	trs := []TR{
		{Number: "NPLK<script>", Description: "<script>alert(1)</script>", Owner: "User&Admin"},
	}
	var b strings.Builder
	for _, tr := range trs {
		fmt.Fprintf(&b, "<option value=%q>%s \xe2\x80\x94 %s (%s)</option>\n",
			tr.Number, tr.Number, tr.Description, tr.Owner)
	}
	fmt.Println(b.String())
}
