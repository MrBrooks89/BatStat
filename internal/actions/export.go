package actions

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MrBrooks89/BatStat/internal/models"
)

func ExportToCSV(connections []models.Connection) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter export filename (leave blank for BatStat_export.csv): ")
	userInput, _ := reader.ReadString('\n')
	userInput = strings.TrimSpace(userInput)

	// Default filename
	filename := "BatStat_export.csv"

	if userInput != "" {
		// Ensure .csv extension
		if !strings.HasSuffix(userInput, ".csv") {
			userInput += ".csv"
		}

		// Resolve relative/absolute path
		absPath, err := filepath.Abs(userInput)
		if err != nil {
			return "", err
		}
		filename = absPath
	}

	// Check if file exists
	if _, err := os.Stat(filename); err == nil {
		// File exists, ask for overwrite
		for {
			fmt.Printf("File %s already exists. Overwrite? (y/n): ", filename)
			overwriteInput, _ := reader.ReadString('\n')
			overwriteInput = strings.TrimSpace(strings.ToLower(overwriteInput))
			if overwriteInput == "y" || overwriteInput == "yes" {
				break
			} else if overwriteInput == "n" || overwriteInput == "no" {
				fmt.Println("Export cancelled.")
				return "", nil
			} else {
				fmt.Println("Please enter 'y' or 'n'.")
			}
		}
	}

	// Create file
	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header row
	headers := []string{"ProcessName", "PID", "Status", "Family", "Type", "LocalAddr", "RemoteAddr"}
	if err := writer.Write(headers); err != nil {
		return "", err
	}

	// Write data rows
	for _, c := range connections {
		row := []string{
			c.ProcessName,
			strconv.Itoa(int(c.Pid)),
			c.Status,
			c.Family,
			c.Type,
			c.Laddr,
			c.Raddr,
		}
		if err := writer.Write(row); err != nil {
			return "", err
		}
	}

	return filename, nil
}

