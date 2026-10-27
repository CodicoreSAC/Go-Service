package kardex

import (
	"testing"

	"kardex-pdf-service/internal/config"
)

func TestCalculateMetrics(t *testing.T) {
	cfg := config.Default()
	gen := NewGenerator(Institution{RazonSocial: "Test", RUC: "12345678"}, "Periodo: Test", cfg, "L", "simple", false)

	insumos := []KardexInsumo{
		{
			Kardex: KardexData{
				Movimientos: make([]KardexMovimiento, 5),
			},
		},
		{
			Kardex: KardexData{
				Movimientos: make([]KardexMovimiento, 3),
			},
		},
		{
			Kardex: KardexData{
				Movimientos: []KardexMovimiento{}, // 0 movimientos
			},
		},
	}

	metrics := gen.CalculateMetrics(insumos)

	if metrics.TotalItems != 3 {
		t.Errorf("TotalItems = %d, want 3", metrics.TotalItems)
	}
	if metrics.TotalMovements != 8 {
		t.Errorf("TotalMovements = %d, want 8", metrics.TotalMovements)
	}
}

func TestCalculateMetricsEmpty(t *testing.T) {
	cfg := config.Default()
	gen := NewGenerator(Institution{}, "", cfg, "L", "simple", false)

	metrics := gen.CalculateMetrics([]KardexInsumo{})

	if metrics.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", metrics.TotalItems)
	}
	if metrics.TotalMovements != 0 {
		t.Errorf("TotalMovements = %d, want 0", metrics.TotalMovements)
	}
}

func TestCreateOptimizedBatches(t *testing.T) {
	cfg := config.Default()
	cfg.ItemsPerBatch = 2 // 2 insumos por batch para test

	gen := NewGenerator(Institution{}, "", cfg, "L", "", false)

	insumos := []KardexInsumo{
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 10)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 20)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 30)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 40)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 50)}},
	}

	batches := gen.createOptimizedBatches(insumos)

	// 5 insumos / 2 per batch = 3 batches (2, 2, 1)
	if len(batches) != 3 {
		t.Errorf("Expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 2 {
		t.Errorf("Batch 0 should have 2 items, got %d", len(batches[0]))
	}
	if len(batches[1]) != 2 {
		t.Errorf("Batch 1 should have 2 items, got %d", len(batches[1]))
	}
	if len(batches[2]) != 1 {
		t.Errorf("Batch 2 should have 1 item, got %d", len(batches[2]))
	}
}

func TestCreateOptimizedBatchesMovementLimit(t *testing.T) {
	cfg := config.Default()
	cfg.ItemsPerBatch = 100
	cfg.MaxMovementsPerBatch = 50

	gen := NewGenerator(Institution{}, "", cfg, "L", "", false)

	insumos := []KardexInsumo{
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 30)}},
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 30)}}, // 60 total > 50, split
		{Kardex: KardexData{Movimientos: make([]KardexMovimiento, 10)}},
	}

	batches := gen.createOptimizedBatches(insumos)

	// First batch: insumo 0 (30 mov) — fits (empty batch)
	// Second batch: insumo 1 (30 mov) — empty batch, fits alone
	// Still in batch 2: insumo 2 (10 mov) — 30+10=40 < 50, fits
	// Result: 2 batches [0], [1, 2]
	if len(batches) != 2 {
		t.Errorf("Expected 2 batches (movement limit), got %d", len(batches))
	}
}

func TestTotalPagesInitiallyZero(t *testing.T) {
	cfg := config.Default()
	gen := NewGenerator(Institution{}, "", cfg, "L", "", false)

	if gen.TotalPages() != 0 {
		t.Errorf("TotalPages() = %d, want 0 initially", gen.TotalPages())
	}
}

// generateSingleFile necesita filesystem — ese es test de integración (test-go-service.sh)
