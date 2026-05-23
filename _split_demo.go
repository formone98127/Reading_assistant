package main
import (
  "fmt"
  "reading-assistant/internal/sentences"
)
func main() {
  samples := []string{
    "Mr. Smith met Mrs. Jones. They left.",
    "MR. AND MRS. DURSLEY",
    "Hello Mr. and Mrs. World. Bye.",
    "Mr.\nSmith went home.",
  }
  for _, s := range samples {
    fmt.Println("IN:", s)
    for i, p := range sentences.Split(s) {
      fmt.Printf("  %d: %q\n", i, p)
    }
  }
}
