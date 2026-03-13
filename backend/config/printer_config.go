package config

import (
	"backend/services/printing"
	"fmt"
	"os"
)

// LoadPrinterConfig reads printer settings from environment variables.
func LoadPrinterConfig() *printing.PrinterConfig {
	connType := os.Getenv("PRINTER_TYPE")
	if connType == "" {
		connType = "network"
	}

	address := os.Getenv("PRINTER_ADDRESS")

	paperWidth := printing.Paper80mm // default for WP230W
	if pw := os.Getenv("PRINTER_PAPER_WIDTH"); pw != "" {
		fmt.Sscanf(pw, "%d", &paperWidth)
	}

	return &printing.PrinterConfig{
		Type:       printing.ConnectionType(connType),
		Address:    address,
		PaperWidth: paperWidth,
	}
}
