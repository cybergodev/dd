//go:build examples

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/cybergodev/dd"
)

// ComplexStruct Example struct with various field types
type ComplexStruct struct {
	Name         string
	Age          int
	IsActive     bool
	CreatedAt    time.Time
	Tags         []string
	Metadata     map[string]any
	privateField string // unexported field
}

// CustomID Custom type implementing Stringer interface
type CustomID string

func (c CustomID) String() string {
	return fmt.Sprintf("ID-%s", string(c))
}

func main() {
	fmt.Println("=== DD Intelligent Type Conversion Examples ===\n ")

	// 1. Simple types
	fmt.Println("1. Simple Types:")
	dd.Text("String:", "Hello World")
	dd.Text("Integer:", 42)
	dd.Text("Float:", 3.14159)
	dd.Text("Boolean:", true)
	dd.Text("Nil:", nil)

	// 2. Complex types
	fmt.Println("\n2. Complex Types:")
	complexStruct := ComplexStruct{
		Name:      "John Doe",
		Age:       30,
		IsActive:  true,
		CreatedAt: time.Now(),
		Tags:      []string{"developer", "golang", "backend"},
		Metadata: map[string]any{
			"level":       "senior",
			"years":       5,
			"remote":      true,
			"skills":      []string{"Go", "Python", "Docker"},
			"preferences": map[string]string{"editor": "vscode", "os": "linux"},
		},
		privateField: "hidden",
	}
	dd.Text("Complex Struct:", complexStruct)

	// 3. Pointers and nil pointers
	fmt.Println("\n3. Pointers:")
	ptr := &complexStruct
	dd.Text("Pointer to struct:", ptr)

	var nilPtr *ComplexStruct
	dd.Text("Nil pointer:", nilPtr)

	// 4. Functions and channels (unmarshalable types)
	fmt.Println("\n4. Unmarshalable Types:")
	testFunc := func(x int) string { return fmt.Sprintf("result: %d", x) }
	dd.Text("Function:", testFunc)

	testChan := make(chan int, 5)
	dd.Text("Channel:", testChan)

	// 5. Error types
	fmt.Println("\n5. Error Types:")
	var nilErr error
	dd.Text("Nil error:", nilErr)

	customErr := errors.New("something went wrong")
	dd.Text("Custom error:", customErr)

	// 6. Time and duration
	fmt.Println("\n6. Time and Duration:")
	now := time.Now()
	duration := time.Hour*2 + time.Minute*30
	dd.Text("Current time:", now)
	dd.Text("Duration:", duration)

	// 7. Complex numbers
	fmt.Println("\n7. Complex Numbers:")
	complexNum := complex(3.14, 2.71)
	dd.Text("Complex number:", complexNum)

	// 8. Custom types with String() method
	fmt.Println("\n8. Custom Types:")
	customID := CustomID("12345")
	dd.Text("Custom ID:", customID)

	// 9. Mixed data in maps and slices
	fmt.Println("\n9. Mixed Data Structures:")
	mixedData := map[string]any{
		"string":   "text",
		"number":   42,
		"boolean":  true,
		"nil":      nil,
		"slice":    []any{1, "two", 3.0, true},
		"map":      map[string]int{"a": 1, "b": 2},
		"struct":   complexStruct,
		"function": testFunc,
		"channel":  testChan,
		"error":    customErr,
		"time":     now,
		"duration": duration,
		"complex":  complexNum,
		"custom":   customID,
	}
	dd.Text("Mixed data:", mixedData)

	// 10. JSON vs Text output comparison
	fmt.Println("\n10. JSON vs Text Comparison:")
	fmt.Println("JSON format:")
	dd.Json(mixedData)

	fmt.Println("\nText format (pretty-printed):")
	dd.Text(mixedData)

	// 11. Circular reference handling
	fmt.Println("\n11. Circular Reference Handling:")
	type Node struct {
		Value string
		Next  *Node
	}

	node1 := &Node{Value: "first"}
	node2 := &Node{Value: "second"}
	node1.Next = node2
	node2.Next = node1 // Create circular reference

	dd.Text("Circular reference:", node1)

	fmt.Println("\n=== All examples completed successfully! ===")
}
