package actions

import (
	"encoding/csv"
	"os"
	"strconv"

	"github.com/MrBrooks89/BatStat/internal/models"
)

func ExportToCSV(connections []models.Connection, filename string) (string, error) {
	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{"ProcessName", "PID", "Status", "Family", "Type", "LocalAddr", "RemoteAddr"}
	if err := writer.Write(headers); err != nil {
		return "", err
	}

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
