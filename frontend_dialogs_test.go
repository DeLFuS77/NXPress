package main

import (
	"os"
	"testing"

	"golang.org/x/net/html"
)

// Every dialog of the page hangs from the body: one left open in the markup takes the dialogs
// after it inside, and they open hidden in it, blocking the window (#121).
func TestDialogsHangFromTheBody(t *testing.T) {
	file, err := os.Open("frontend/index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	page, err := html.Parse(file)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "dialog" {
			found++
			if node.Parent == nil || node.Parent.Data != "body" {
				id := ""
				for _, attribute := range node.Attr {
					if attribute.Key == "id" {
						id = attribute.Val
					}
				}
				parent := ""
				if node.Parent != nil {
					parent = node.Parent.Data
					for _, attribute := range node.Parent.Attr {
						if attribute.Key == "id" {
							parent += "#" + attribute.Val
						}
					}
				}
				t.Errorf("dialog %q is inside %s, not the body", id, parent)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(page)
	if found == 0 {
		t.Fatal("no dialog found in the page")
	}
}
