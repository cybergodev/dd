package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("=== DD Message Size Limit ===\n ")

	defaultLimit()
	customLimit()

	fmt.Println("\n✅ Examples completed")
	fmt.Println("\nNote: Default limit is 5MB, customizable via SecurityConfig.MaxMessageSize")
}

// 1. Default Limit - 5MB maximum message size
func defaultLimit() {
	fmt.Println("1. Default Limit (5MB)")

	var buf bytes.Buffer
	config := dd.DefaultConfig()
	config.Writers = []io.Writer{&buf}
	logger, _ := dd.New(config)

	// Try to log 6MB message
	largeMsg := strings.Repeat("A", 6*1024*1024)
	logger.Info(largeMsg)

	output := buf.String()
	if strings.Contains(output, "[TRUNCATED]") {
		fmt.Printf("  ✓ 6MB message truncated (output: %d bytes)\n", len(output))
	} else {
		fmt.Printf("  ✗ Not truncated\n")
	}
}

// 2. Custom Limit - Set your own size limit
func customLimit() {
	fmt.Println("\n2. Custom Limit")

	// 1MB limit
	var buf1 bytes.Buffer
	config1 := dd.DefaultConfig()
	config1.Writers = []io.Writer{&buf1}
	config1.SecurityConfig = &dd.SecurityConfig{
		MaxMessageSize: 1 * 1024 * 1024, // 1MB
	}
	logger1, _ := dd.New(config1)

	largeMsg1 := strings.Repeat("B", 2*1024*1024)
	logger1.Info(largeMsg1)

	if strings.Contains(buf1.String(), "[TRUNCATED]") {
		fmt.Printf("  ✓ 2MB message truncated to 1MB (output: %d bytes)\n", len(buf1.String()))
	}

	// 10MB limit
	var buf2 bytes.Buffer
	config2 := dd.DefaultConfig()
	config2.Writers = []io.Writer{&buf2}
	config2.SecurityConfig = &dd.SecurityConfig{
		MaxMessageSize: 10 * 1024 * 1024, // 10MB
	}
	logger2, _ := dd.New(config2)

	largeMsg2 := strings.Repeat("C", 6*1024*1024)
	logger2.Info(largeMsg2)

	if !strings.Contains(buf2.String(), "[TRUNCATED]") {
		fmt.Printf("  ✓ 6MB message not truncated (output: %d bytes)\n", len(buf2.String()))
	}
}
