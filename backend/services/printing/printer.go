package printing

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// ============================================================================
// PRINTER SERVICE — Winpal WP230W (USB + WiFi/Network)
// ============================================================================

// ConnectionType determines how we connect to the printer.
type ConnectionType string

const (
	ConnUSB     ConnectionType = "usb"
	ConnNetwork ConnectionType = "network"
)

// PrinterConfig holds printer connection settings.
type PrinterConfig struct {
	Type       ConnectionType // "usb" or "network"
	Address    string         // For network: "192.168.1.100:9100"; for USB: device path (e.g., "/dev/usb/lp0" or "\\\\.\COM3")
	PaperWidth int            // 58 or 80 (mm)
}

// PrinterService manages the connection to the receipt printer.
type PrinterService struct {
	config *PrinterConfig
}

// NewPrinterService creates a PrinterService from config.
func NewPrinterService(cfg *PrinterConfig) *PrinterService {
	if cfg == nil {
		log.Println("⚠️  PrinterService: no printer configured")
		cfg = &PrinterConfig{
			Type:       ConnNetwork,
			Address:    "",
			PaperWidth: Paper80mm,
		}
	}
	return &PrinterService{config: cfg}
}

// PrintReceipt formats the receipt and sends it to the printer.
func (p *PrinterService) PrintReceipt(receipt *Receipt) error {
	if p.config.Address == "" {
		return fmt.Errorf("printer not configured: address is empty")
	}

	// Generate ESC/POS bytes
	data := FormatReceipt(receipt, p.config.PaperWidth)

	// Send to printer
	if err := p.sendData(data); err != nil {
		return fmt.Errorf("print failed: %w", err)
	}

	log.Printf("✅ PrinterService: receipt printed for order %s (%d bytes sent)",
		receipt.OrderNumber, len(data))
	return nil
}

// OpenCashDrawer sends the cash drawer kick pulse.
func (p *PrinterService) OpenCashDrawer() error {
	if p.config.Address == "" {
		return fmt.Errorf("printer not configured: address is empty")
	}
	return p.sendData(CmdOpenDrawer)
}

// TestConnection checks if the printer is reachable.
func (p *PrinterService) TestConnection() error {
	if p.config.Address == "" {
		return fmt.Errorf("printer not configured: address is empty")
	}

	switch p.config.Type {
	case ConnNetwork:
		conn, err := net.DialTimeout("tcp", p.config.Address, 5*time.Second)
		if err != nil {
			return fmt.Errorf("network printer unreachable at %s: %w", p.config.Address, err)
		}
		conn.Close()
		return nil

	case ConnUSB:
		_, err := os.Stat(p.config.Address)
		if err != nil {
			return fmt.Errorf("USB printer device not found at %s: %w", p.config.Address, err)
		}
		return nil

	default:
		return fmt.Errorf("unknown connection type: %s", p.config.Type)
	}
}

// GetConfig returns the current printer configuration.
func (p *PrinterService) GetConfig() *PrinterConfig {
	return p.config
}

// ── Internal ─────────────────────────────────────────────────────────────────

func (p *PrinterService) sendData(data []byte) error {
	switch p.config.Type {
	case ConnNetwork:
		return p.sendViaNetwork(data)
	case ConnUSB:
		return p.sendViaUSB(data)
	default:
		return fmt.Errorf("unknown connection type: %s", p.config.Type)
	}
}

func (p *PrinterService) sendViaNetwork(data []byte) error {
	conn, err := net.DialTimeout("tcp", p.config.Address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", p.config.Address, err)
	}
	defer conn.Close()

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	written, err := conn.Write(data)
	if err != nil {
		return fmt.Errorf("write to printer: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("incomplete write: %d/%d bytes", written, len(data))
	}

	return nil
}

func (p *PrinterService) sendViaUSB(data []byte) error {
	// On Windows: \\.\COM3 or \\.\USB001
	// On Linux/Mac: /dev/usb/lp0
	file, err := os.OpenFile(p.config.Address, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open USB device %s: %w", p.config.Address, err)
	}
	defer file.Close()

	written, err := file.Write(data)
	if err != nil {
		return fmt.Errorf("write to USB device: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("incomplete USB write: %d/%d bytes", written, len(data))
	}

	return nil
}
