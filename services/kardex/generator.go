package kardex

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jung-kurt/gofpdf"

	"kardex-pdf-service/internal/config"
	"kardex-pdf-service/internal/shared"
	"kardex-pdf-service/internal/tracker"
)

// Generator generador optimizado de PDF para Kardex.
type Generator struct {
	institution  Institution
	totalPages   int
	timestamp    string
	periodo      string
	totalMovements int
	startTime    time.Time
	cfg          *config.AppConfig
	orientation  string      // "L" (landscape) o "P" (portrait)
	style        TableStyle  // estilo visual de las tablas
}

// NewGenerator crea un nuevo generador.
func NewGenerator(institution Institution, periodo string, cfg *config.AppConfig, orientation string, styleName string) *Generator {
	if orientation == "" {
		orientation = "L" // default: landscape
	}
	return &Generator{
		institution:  institution,
		totalPages:   0,
		timestamp:    time.Now().Format("02/01/2006 15:04:05"),
		periodo:      periodo,
		startTime:    time.Now(),
		cfg:          cfg,
		orientation:  orientation,
		style:        GetStyleByName(styleName),
	}
}

// TotalPages retorna el total de páginas generadas.
func (g *Generator) TotalPages() int {
	return g.totalPages
}

// CalculateMetrics calcula las métricas del dataset.
func (g *Generator) CalculateMetrics(insumos []KardexInsumo) Metrics {
	totalMovements := 0
	for i := range insumos {
		totalMovements += len(insumos[i].Kardex.Movimientos)
	}
	g.totalMovements = totalMovements
	return Metrics{
		TotalItems:     len(insumos),
		TotalMovements: totalMovements,
	}
}

// Generate genera el PDF con estrategia adaptativa (kardex completo).
func (g *Generator) Generate(insumos []KardexInsumo, filenameBase, outputFolder string, processKey string, trk *tracker.ProcessTracker) (string, error) {
	if err := os.MkdirAll(outputFolder, 0755); err != nil {
		return "", fmt.Errorf("error creando directorio: %v", err)
	}

	metrics := g.CalculateMetrics(insumos)
	log.Printf("[KardexPDF] Items: %d, Movimientos: %d", metrics.TotalItems, metrics.TotalMovements)

	// Elegir estrategia
	if metrics.TotalMovements <= g.cfg.SingleFileThreshold {
		log.Printf("[KardexPDF] Estrategia: Single File")
		return g.generateSingleFile(insumos, filenameBase, outputFolder, processKey, trk)
	}

	if g.cfg.UseZip && metrics.TotalMovements > g.cfg.MergeThreshold {
		log.Printf("[KardexPDF] Estrategia: ZIP Bundle")
		return g.generateZipBundle(insumos, filenameBase, outputFolder, processKey, trk)
	}

	log.Printf("[KardexPDF] Estrategia: Batch → Single PDF")
	return g.generateBatchPDF(insumos, filenameBase, outputFolder, processKey, trk)
}

// =============================================================================
// RESUMEN DE SALDOS (sin detalle de movimientos)
// =============================================================================

// GenerateSummary genera un PDF con solo portada + tabla de resumen de saldos.
// Mucho más rápido y liviano que el completo — no procesa movimientos.
func (g *Generator) GenerateSummary(insumos []KardexInsumo, filenameBase, outputFolder string, processKey string, trk *tracker.ProcessTracker) (string, error) {
	if err := os.MkdirAll(outputFolder, 0755); err != nil {
		return "", fmt.Errorf("error creando directorio: %v", err)
	}

	metrics := g.CalculateMetrics(insumos)
	log.Printf("[KardexPDF-Resumen] Items: %d, Movimientos: %d (no se procesan)", metrics.TotalItems, metrics.TotalMovements)

	trk.Update(processKey, 10, "in-progress", "Generando resumen de saldos...")

	pdf := g.createNewPDF()

	// Header diferente para resumen
	pdf.SetHeaderFunc(func() {
		pdf.SetY(3)
		pdf.SetFont("Arial", "B", 8)
		pdf.CellFormat(0, 4, "RESUMEN DE SALDOS - "+shared.ToLatin(g.institution.RazonSocial), "", 1, "C", false, 0, "")
		pdf.SetFont("Arial", "", 7)
		pdf.CellFormat(0, 3.5, shared.ToLatin(g.periodo), "", 1, "C", false, 0, "")
		pdf.Ln(1.5)
	})

	g.addCoverPage(pdf)

	// Cambiar título de portada para resumen
	// (la portada ya se dibujó, pero el header del PDF ahora dice "RESUMEN DE SALDOS")

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 6, "RESUMEN DE SALDOS", "", 1, "C", false, 0, "")

	trk.Update(processKey, 30, "in-progress", "Generando tabla de resumen...")
	g.addProductSummaryPage(pdf, insumos)

	trk.Update(processKey, 90, "in-progress", "Guardando PDF...")

	filename := filenameBase + ".pdf"
	outputPath := filepath.Join(outputFolder, filename)

	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("error creando archivo: %v", err)
	}

	if err := pdf.Output(file); err != nil {
		file.Close()
		return "", fmt.Errorf("error escribiendo PDF: %v", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("error sincronizando archivo: %v", err)
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("error cerrando archivo: %v", err)
	}

	trk.Update(processKey, 100, "completed", "Resumen generado exitosamente")
	g.totalPages = pdf.PageCount()

	log.Printf("[KardexPDF-Resumen] ✅ PDF generado: %s (%d páginas)", filename, g.totalPages)
	return filename, nil
}

// =============================================================================
// FORMATO 13.1 SUNAT — Registro del Inventario Permanente Valorizado
// =============================================================================

// GenerateSUNAT13 genera el PDF con formato oficial SUNAT 13.1.
// Sin portada, sin resumen — solo tablas de detalle por producto.
// Headers obligatorios con datos del contribuyente.
func (g *Generator) GenerateSUNAT13(insumos []KardexInsumo, filenameBase, outputFolder string, processKey string, trk *tracker.ProcessTracker) (string, error) {
	if err := os.MkdirAll(outputFolder, 0755); err != nil {
		return "", fmt.Errorf("error creando directorio: %v", err)
	}

	metrics := g.CalculateMetrics(insumos)
	log.Printf("[KardexPDF-SUNAT13.1] Items: %d, Movimientos: %d", metrics.TotalItems, metrics.TotalMovements)

	trk.Update(processKey, 5, "in-progress", "Generando Formato 13.1 SUNAT...")

	pdf := g.createNewPDF()

	// Header específico SUNAT 13.1
	pdf.SetHeaderFunc(func() {
		pdf.SetY(3)
		pdf.SetFont("Arial", "B", 8)
		pdf.CellFormat(0, 4, "FORMATO 13.1 - REGISTRO DEL INVENTARIO PERMANENTE VALORIZADO", "", 1, "C", false, 0, "")
		pdf.SetFont("Arial", "", 6.5)
		pdf.CellFormat(0, 3.5, "Periodo: "+shared.ToLatin(g.periodo)+" | RUC: "+g.institution.RUC+" | "+shared.ToLatin(g.institution.RazonSocial), "", 1, "C", false, 0, "")
		pdf.Ln(1.5)
	})

	pdf.AddPage()

	// Datos del contribuyente (SUNAT exige esto al inicio)
	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(0, 5, "IDENTIFICACION DEL CONTRIBUYENTE", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 7)

	leftMargin, _, _, _ := pdf.GetMargins()
	labelW := 40.0
	valueW := 80.0
	_, pageWidth := pdf.GetPageSize()
	rightValueW := pageWidth - leftMargin*2 - labelW*2 - valueW

	pdf.CellFormat(labelW, 4, "RUC:", "1", 0, "R", true, 0, "")
	pdf.CellFormat(valueW, 4, " "+g.institution.RUC, "1", 0, "L", false, 0, "")
	pdf.CellFormat(labelW, 4, "Razon Social:", "1", 0, "R", true, 0, "")
	pdf.CellFormat(rightValueW, 4, " "+shared.ToLatin(g.institution.RazonSocial), "1", 0, "L", false, 0, "")
	pdf.Ln(-1)

	pdf.CellFormat(labelW, 4, "Periodo:", "1", 0, "R", true, 0, "")
	pdf.CellFormat(valueW, 4, " "+shared.ToLatin(g.periodo), "1", 0, "L", false, 0, "")
	pdf.CellFormat(labelW, 4, "Establecimiento:", "1", 0, "R", true, 0, "")
	pdf.CellFormat(rightValueW, 4, " "+shared.ToLatin(g.institution.Address), "1", 0, "L", false, 0, "")
	pdf.Ln(-1)

	pdf.CellFormat(labelW, 4, "Codigo:", "1", 0, "R", true, 0, "")
	pdf.CellFormat(valueW+labelW+rightValueW, 4, " "+shared.ToLatin(g.periodo), "1", 0, "L", false, 0, "")
	pdf.Ln(-1)

	pdf.Ln(5)

	// Detalle por cada insumo
	for i := range insumos {
		trk.Update(processKey, 5+int(float64(i)/float64(len(insumos))*90), "in-progress",
			fmt.Sprintf("Generando producto %d/%d...", i+1, len(insumos)))

		g.addSUNAT13InsumoTable(pdf, &insumos[i], i+1, len(insumos))
	}

	trk.Update(processKey, 95, "in-progress", "Guardando PDF...")

	filename := filenameBase + ".pdf"
	outputPath := filepath.Join(outputFolder, filename)

	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("error creando archivo: %v", err)
	}
	if err := pdf.Output(file); err != nil {
		file.Close()
		return "", fmt.Errorf("error escribiendo PDF: %v", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("error sincronizando archivo: %v", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("error cerrando archivo: %v", err)
	}

	trk.Update(processKey, 100, "completed", "Formato 13.1 generado exitosamente")
	g.totalPages = pdf.PageCount()

	log.Printf("[KardexPDF-SUNAT13.1] ✅ PDF generado: %s (%d páginas)", filename, g.totalPages)
	return filename, nil
}

// =============================================================================
// GENERATE ALL — 3 formatos en 1 pasada (JSON cargado 1 sola vez)
// =============================================================================

// FormatSpec define un formato a generar.
type FormatSpec struct {
	ReportType string
	Suffix      string // sufijo para el filename
	Orientation string // "L" o "P"
	Style       string // "simple" | "formal" | "contador"
}

// defaultFormats son los 3 formatos que se generan por defecto.
var defaultFormats = []FormatSpec{
	{ReportType: "completo", Suffix: "-completo", Orientation: "L"},
	{ReportType: "resumen_saldos", Suffix: "-resumen-saldos", Orientation: "L"},
	{ReportType: "sunat_13_1", Suffix: "-formato-13.1", Orientation: "P"},
}

// GenerateAll genera los 3 formatos en una sola pasada.
// Cada formato se genera con su propio Generator (orientación propia).
// Si uno falla, los otros continúan.
// El progreso se calcula como: (formato_inicio + %dentro_del_formato) / total_formatos.
func GenerateAll(
	insumos []KardexInsumo,
	institution Institution,
	periodo, filenameBase, outputFolder string,
	processKey string,
	trk *tracker.ProcessTracker,
	cfg *config.AppConfig,
	userOrientation string,
	userStyle string,
) []GenerateAllResult {
	if err := os.MkdirAll(outputFolder, 0755); err != nil {
		return []GenerateAllResult{
			{ReportType: "all", Error: fmt.Sprintf("error creando directorio: %v", err)},
		}
	}

	// Inyectar el style del usuario en todos los formatos
	formats := make([]FormatSpec, len(defaultFormats))
	copy(formats, defaultFormats)
	for i := range formats {
		if userStyle != "" {
			formats[i].Style = userStyle
		}
	}

	totalFormats := len(formats)
	totalItems := len(insumos)
	results := make([]GenerateAllResult, 0, totalFormats)

	// Métricas iniciales
	trk.Update(processKey, 2, "in-progress", fmt.Sprintf("Iniciando generación de %d formatos para %d productos...", totalFormats, totalItems))

	for i, spec := range formats {
		step := i + 1
		formatStartPct := (i * 100) / totalFormats // 0, 33, 66

		// Notificar inicio de este formato
		label := fmt.Sprintf("Formato %d/%d: %s", step, totalFormats, spec.ReportType)
		trk.Update(processKey, formatStartPct+1, "in-progress", label)

		// Crear generator: si el usuario eligió orientación, usar esa; sino default del formato
		orientation := spec.Orientation
		if userOrientation == "P" {
			orientation = "P"
		}
		gen := NewGenerator(institution, periodo, cfg, orientation, spec.Style)
		gen.totalMovements = 0 // reset

		filenameSuffix := filenameBase + spec.Suffix
		var filename string
		var err error

		switch spec.ReportType {
		case "completo":
			filename, err = gen.Generate(insumos, filenameSuffix, outputFolder, processKey, trk)
		case "resumen_saldos":
			filename, err = gen.GenerateSummary(insumos, filenameSuffix, outputFolder, processKey, trk)
		case "sunat_13_1":
			filename, err = gen.GenerateSUNAT13(insumos, filenameSuffix, outputFolder, processKey, trk)
		}

		// Calcular % final de este formato
		formatEndPct := ((i + 1) * 100) / totalFormats // 33, 66, 100

		result := GenerateAllResult{
			ReportType: spec.ReportType,
			TotalPages: gen.TotalPages(),
		}

		if err != nil {
			result.Error = err.Error()
			trk.Update(processKey, formatEndPct, "in-progress",
				fmt.Sprintf("❌ %s falló (%d/%d)", spec.ReportType, step, totalFormats))
			log.Printf("[KardexPDF-All] ❌ %s falló: %v", spec.ReportType, err)
		} else {
			result.Filename = filename
			trk.Update(processKey, formatEndPct, "in-progress",
				fmt.Sprintf("✅ %s listo (%d/%d) — %d páginas, %d items", spec.ReportType, step, totalFormats, gen.TotalPages(), totalItems))
			log.Printf("[KardexPDF-All] ✅ %s generado: %s (%d páginas)", spec.ReportType, filename, result.TotalPages)
		}

		results = append(results, result)
	}

	// Contar éxitos/fallos para el mensaje final
	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Error == "" {
			successCount++
		} else {
			failCount++
		}
	}

	finalMsg := fmt.Sprintf("Completado: %d exitosos, %d fallidos de %d formatos (%d productos)",
		successCount, failCount, totalFormats, totalItems)

	trk.Update(processKey, 100, "completed", finalMsg)
	return results
}

// addSUNAT13InsumoTable agrega la tabla de detalle de un producto en formato 13.1.
func (g *Generator) addSUNAT13InsumoTable(pdf *gofpdf.Fpdf, insumo *KardexInsumo, index, total int) {
	denomination := "Sin denominacion"
	barcode := "S/C"
	if insumo.InsumoDetail.Insumo != nil {
		if insumo.InsumoDetail.Insumo.Denomination != "" {
			denomination = shared.ToLatin(insumo.InsumoDetail.Insumo.Denomination)
		}
		if insumo.InsumoDetail.Insumo.Barcode != "" {
			barcode = shared.ToLatin(insumo.InsumoDetail.Insumo.Barcode)
		}
	}

	// Título del producto
	pdf.SetFont("Arial", "B", 7)
	pdf.CellFormat(0, 5, fmt.Sprintf("Producto %d/%d: %s (%s)", index, total, denomination, barcode), "", 1, "L", false, 0, "")

	// Columnas SUNAT 13.1: 12 columnas (más compactas que el completo)
	// Fecha | Tipo Doc | Serie | Numero | Entradas(Cant,P.Unit,Total) | Salidas(Cant,P.Unit,Total) | Saldo(Cant,P.Unit,Total)
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	pageWidth, _ := pdf.GetPageSize()
	available := pageWidth - leftMargin - rightMargin

	// Ratios suman 100% — las celdas ocupan todo el ancho disponible.
	ratios := []float64{6.78, 6.21, 4.52, 6.78, 6.21, 7.34, 7.34, 8.47, 7.34, 7.34, 8.47, 7.34, 7.34, 8.48}
	cw := make([]float64, 14)
	for i, r := range ratios {
		cw[i] = (r / 100.0) * available
	}

	maxY := g.maxContentY(pdf)

	// Header SUNAT
	pdf.SetFont("Arial", "B", 5.5)
	g.style.ApplyHeader(pdf)
	hbf := g.style.HeaderBorderFlag()

	// Fila 1: colspanes
	pdf.CellFormat(cw[0], 3.5, "Fecha", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[1]+cw[2]+cw[3], 3.5, "Comprobante", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[4], 3.5, "T. Oper.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[5]+cw[6]+cw[7], 3.5, "Entradas", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[8]+cw[9]+cw[10], 3.5, "Salidas", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[11]+cw[12]+cw[13], 3.5, "Saldo Final", hbf, 0, "C", true, 0, "")
	pdf.Ln(-1)

	// Fila 2: sub-headers
	g.style.ApplyHeader(pdf)
	pdf.CellFormat(cw[0], 3.5, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[1], 3.5, "Tipo", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[2], 3.5, "Serie", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[3], 3.5, "Numero", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[4], 3.5, "", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[5], 3.5, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[6], 3.5, "P.Unit.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[7], 3.5, "Total", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[8], 3.5, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[9], 3.5, "P.Unit.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[10], 3.5, "Total", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[11], 3.5, "Cant.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[12], 3.5, "P.Unit.", hbf, 0, "C", true, 0, "")
	pdf.CellFormat(cw[13], 3.5, "Total", hbf, 0, "C", true, 0, "")
	pdf.Ln(-1)

	pdf.SetFont("Arial", "", 5.5)

	// Saldo inicial
	if insumo.Kardex.Initial != nil {
		g.style.ApplySaldoInit(pdf)
		x := leftMargin
		y := pdf.GetY()
		rh := rowHeight(1)

		cx := x
		rectFlag := g.style.DataRectFlag()
		for j := 0; j < 14; j++ {
			pdf.Rect(cx, y, cw[j], rh, rectFlag)
			cx += cw[j]
		}

		pdf.SetFont("Arial", "B", 5.5)
		pdf.SetXY(x+cw[0]+cw[1]+cw[2]+cw[3]+cellGap, y+cellGap)
		pdf.CellFormat(cw[4]-cellGap*2, lineHt, "S. INICIAL", "", 0, "C", false, 0, "")
		pdf.SetFont("Arial", "", 5.5)

		// Saldo inicial va en las columnas de Saldo Final (11,12,13)
		siX := x + cw[0] + cw[1] + cw[2] + cw[3] + cw[4] + cw[5] + cw[6] + cw[7] + cw[8] + cw[9] + cw[10]
		pdf.SetXY(siX+cellGap, y+cellGap)
		pdf.CellFormat(cw[11]-cellGap*2, lineHt, shared.FormatNumber(insumo.Kardex.Initial.Cantidad), "", 0, "R", false, 0, "")
		pdf.SetXY(siX+cw[11]+cellGap, y+cellGap)
		pdf.CellFormat(cw[12]-cellGap*2, lineHt, shared.FormatNumber(insumo.Kardex.Initial.Precio), "", 0, "R", false, 0, "")
		pdf.SetXY(siX+cw[11]+cw[12]+cellGap, y+cellGap)
		pdf.CellFormat(cw[13]-cellGap*2, lineHt, shared.FormatNumber(insumo.Kardex.Initial.Parcial), "", 0, "R", false, 0, "")

		pdf.SetXY(leftMargin, y+rh)
	}

	// Filtrar proformas
	validMovements := make([]KardexMovimiento, 0, len(insumo.Kardex.Movimientos))
	for _, m := range insumo.Kardex.Movimientos {
		if m.Asunto != "proforma" {
			validMovements = append(validMovements, m)
		}
	}

	// Movimientos
	for idx, m := range validMovements {
		rh := rowHeight(1)
		if pdf.GetY()+rh > maxY {
			pdf.AddPage()
			// Re-render headers
			pdf.SetFont("Arial", "B", 7)
			pdf.CellFormat(0, 5, fmt.Sprintf("Producto %d/%d: %s (cont.)", index, total, denomination), "", 1, "L", false, 0, "")
			// Re-render column headers
			pdf.SetFont("Arial", "B", 5.5)
			g.style.ApplyHeader(pdf)
			hbf := g.style.HeaderBorderFlag()
			pdf.CellFormat(cw[0], 3.5, "Fecha", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[1]+cw[2]+cw[3], 3.5, "Comprobante", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[4], 3.5, "T. Oper.", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[5]+cw[6]+cw[7], 3.5, "Entradas", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[8]+cw[9]+cw[10], 3.5, "Salidas", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[11]+cw[12]+cw[13], 3.5, "Saldo Final", hbf, 0, "C", true, 0, "")
			pdf.Ln(-1)
			pdf.CellFormat(cw[0], 3.5, "", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[1], 3.5, "Tipo", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[2], 3.5, "Serie", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[3], 3.5, "Numero", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[4], 3.5, "", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[5], 3.5, "Cant.", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[6], 3.5, "P.Unit.", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[7], 3.5, "Total", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[8], 3.5, "Cant.", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[9], 3.5, "P.Unit.", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[10], 3.5, "Total", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[11], 3.5, "Cant.", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[12], 3.5, "P.Unit.", hbf, 0, "C", true, 0, "")
			pdf.CellFormat(cw[13], 3.5, "Total", hbf, 0, "C", true, 0, "")
			pdf.Ln(-1)
			pdf.SetFont("Arial", "", 5.5)
		}

		isSalida := m.Asunto == "salida"
		tipoOp := shared.ToLatin(m.TipoDocumento)

		var entCant, entPrecio, entTotal, salCant, salPrecio, salTotal string

		if !isSalida {
			entCant = shared.FormatNumber(shared.GetFloatValue(m.CantidadIngreso))
			entPrecio = shared.FormatNumber(shared.GetFloatValue(m.PrecioIngreso))
			entTotal = shared.FormatNumber(shared.GetFloatValue(m.ParcialIngreso))
		}
		if isSalida {
			salCant = shared.FormatNumber(shared.GetFloatValue(m.CantidadSalida))
			salPrecio = shared.FormatNumber(shared.GetFloatValue(m.PrecioSalida))
			salTotal = shared.FormatNumber(shared.GetFloatValue(m.ParcialSalida))
		}

		texts := []string{
			shared.ToLatin(m.FechaKardex),
			shared.ToLatin(m.TipoDocumento),
			shared.ToLatin(m.SerieDocumento),
			shared.ToLatin(m.NumeroDocumento),
			tipoOp,
			entCant, entPrecio, entTotal,
			salCant, salPrecio, salTotal,
			shared.FormatNumber(shared.GetFloatValue(m.CantidadFinal)),
			shared.FormatNumber(shared.GetFloatValue(m.PrecioFinal)),
			shared.FormatNumber(shared.GetFloatValue(m.CostoFinal)),
		}
		aligns := []string{"C", "C", "C", "C", "C", "R", "R", "R", "R", "R", "R", "R", "R", "R"}

		// Aplicar estilo según tipo (salida vs entrada)
		if isSalida {
			g.style.ApplySalida(pdf, idx)
		} else {
			g.style.ApplyData(pdf, idx)
		}

		y := pdf.GetY()
		x := leftMargin
		cx := x
		rectFlag := g.style.DataRectFlag()
		for j, txt := range texts {
			pdf.Rect(cx, y, cw[j], rh, rectFlag)
			if txt != "" {
				pdf.SetXY(cx+cellGap, y+cellGap)
				pdf.CellFormat(cw[j]-cellGap*2, lineHt, txt, "", 0, aligns[j], false, 0, "")
			}
			cx += cw[j]
		}
		pdf.SetXY(leftMargin, y+rh)
	}

	pdf.Ln(5)
}

// =============================================================================
// ESTRATEGIA 1: Archivo único (datasets pequeños)
// =============================================================================

func (g *Generator) generateSingleFile(insumos []KardexInsumo, filenameBase, folder, processKey string, trk *tracker.ProcessTracker) (string, error) {
	trk.Update(processKey, 5, "in-progress", "Calculando totales globales...")
	globalTotals := ComputeGlobalTotals(insumos)

	trk.Update(processKey, 10, "in-progress", "Preparando PDF único")

	pdf := g.createNewPDF()

	// ─── SECCIÓN RESUMEN ───
	// 1. Portada
	g.addCoverPage(pdf)
	// 2. Tabla resumen por producto
	g.addProductSummaryPage(pdf, insumos)
	// 3. Totales globales (TOTAL GENERAL + RESUMEN INVENTARIO + ANALISIS DE COSTOS)
	trk.Update(processKey, 13, "in-progress", "Generando totales globales...")
	g.AddGlobalTotalsSection(pdf, globalTotals)

	// ─── SECCIÓN SEGUIMIENTO ───
	// Margin superior antes de las cabeceras
	pdf.Ln(4)
	// Cabecera de columnas UNA VEZ al inicio de la sección
	seguimientoColWidths := g.kardexColWidths(pdf)
	g.renderKardexHeaderRow1(pdf, seguimientoColWidths)
	pdf.Ln(-1)
	g.renderKardexHeaderRow2(pdf, seguimientoColWidths)
	pdf.Ln(-1)

	// Movimientos por producto + totales por producto (dentro de cada producto)
	totalInsumos := len(insumos)
	for i := range insumos {
		progress := 18 + int(float64(i)/float64(totalInsumos)*77)
		trk.Update(processKey, progress, "in-progress",
			fmt.Sprintf("Procesando insumo %d/%d", i+1, totalInsumos))

		g.addInsumoPage(pdf, &insumos[i], i+1, totalInsumos)
		insumos[i].Kardex.Movimientos = nil
	}

	// Totales globales al FINAL del seguimiento (resumen ejecutivo final)
	pdf.AddPage()
	g.AddGlobalTotalsSection(pdf, globalTotals)

	filename := filenameBase + ".pdf"
	outputPath := filepath.Join(folder, filename)

	if err := pdf.OutputFileAndClose(outputPath); err != nil {
		return "", fmt.Errorf("error guardando PDF: %v", err)
	}

	trk.Update(processKey, 100, "completed", "PDF generado exitosamente")
	g.totalPages = pdf.PageCount()

	return filename, nil
}

// =============================================================================
// ESTRATEGIA 2: Batch → Single PDF (datasets medianos)
// =============================================================================

func (g *Generator) generateBatchPDF(insumos []KardexInsumo, filenameBase, folder, processKey string, trk *tracker.ProcessTracker) (string, error) {
	trk.Update(processKey, 3, "in-progress", "Calculando totales globales...")
	globalTotals := ComputeGlobalTotals(insumos)

	trk.Update(processKey, 5, "in-progress", "Generando PDF único con batches...")

	pdf := g.createNewPDF()

	// Portada y resumen
	trk.Update(processKey, 10, "in-progress", "Agregando portada y resumen...")
	g.addCoverPage(pdf)
	g.addProductSummaryPage(pdf, insumos)
	shared.TriggerGC()

	// Sección de totales globales — después del resumen, antes de los movimientos
	trk.Update(processKey, 13, "in-progress", "Generando totales globales...")
	g.AddGlobalTotalsSection(pdf, globalTotals)

	// Cabecera de columnas UNA VEZ al inicio de la sección de seguimiento
	pdf.Ln(4)
	seguimientoColWidths := g.kardexColWidths(pdf)
	g.renderKardexHeaderRow1(pdf, seguimientoColWidths)
	pdf.Ln(-1)
	g.renderKardexHeaderRow2(pdf, seguimientoColWidths)
	pdf.Ln(-1)

	// Procesar insumos en batches
	batches := g.createOptimizedBatches(insumos)
	startIndex := 0
	totalInsumos := len(insumos)

	for i, batch := range batches {
		progress := 15 + int(float64(i+1)/float64(len(batches))*70)
		trk.Update(processKey, progress, "in-progress",
			fmt.Sprintf("Procesando batch %d/%d", i+1, len(batches)))

		for j := range batch {
			g.addInsumoPage(pdf, &batch[j], startIndex+j+1, totalInsumos)
		}

		for j := range batch {
			batch[j].Kardex.Movimientos = nil
		}
		startIndex += len(batch)
		shared.TriggerGC()

		if (i+1)%10 == 0 {
			shared.LogMemoryUsage(fmt.Sprintf("Batch %d/%d", i+1, len(batches)))
		}
	}

	// Totales globales al FINAL del seguimiento (resumen ejecutivo final)
	pdf.AddPage()
	g.AddGlobalTotalsSection(pdf, globalTotals)

	// Guardar
	trk.Update(processKey, 95, "in-progress", "Guardando PDF...")
	filename := filenameBase + ".pdf"
	outputPath := filepath.Join(folder, filename)

	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("error creando archivo: %v", err)
	}

	if err := pdf.Output(file); err != nil {
		file.Close()
		return "", fmt.Errorf("error escribiendo PDF: %v", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("error sincronizando archivo: %v", err)
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("error cerrando archivo: %v", err)
	}

	trk.Update(processKey, 100, "completed", "PDF generado exitosamente")
	g.totalPages = pdf.PageCount()

	return filename, nil
}

// =============================================================================
// ESTRATEGIA 3: Bundle ZIP (datasets grandes)
// =============================================================================

func (g *Generator) generateZipBundle(insumos []KardexInsumo, filenameBase, folder, processKey string, trk *tracker.ProcessTracker) (string, error) {
	batches := g.createOptimizedBatches(insumos)
	tempDir := filepath.Join(folder, "temp_"+filenameBase)

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("error creando directorio temporal: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Calcular totales globales antes de procesar (movimientos se liberan después)
	trk.Update(processKey, 1, "in-progress", "Calculando totales globales...")
	globalTotals := ComputeGlobalTotals(insumos)

	pdfPaths := make([]string, 0, len(batches)+1)

	// PDF de resumen
	trk.Update(processKey, 2, "in-progress", "Generando resumen...")
	summaryPath := filepath.Join(tempDir, fmt.Sprintf("%s_00_resumen.pdf", filenameBase))
	if err := g.generateSummaryPDF(insumos, summaryPath); err != nil {
		return "", err
	}
	pdfPaths = append(pdfPaths, summaryPath)
	shared.TriggerGC()

	// PDFs por batch
	startIndex := 0
	totalInsumos := len(insumos)

	for i, batch := range batches {
		progress := 5 + int(float64(i+1)/float64(len(batches))*70)
		trk.Update(processKey, progress, "in-progress",
			fmt.Sprintf("Generando batch %d/%d", i+1, len(batches)))

		batchPath := filepath.Join(tempDir, fmt.Sprintf("%s_%02d_insumos_%d-%d.pdf",
			filenameBase, i+1, startIndex+1, startIndex+len(batch)))

		if err := g.generateBatchOnlyPDF(batch, startIndex, totalInsumos, batchPath); err != nil {
			return "", err
		}
		pdfPaths = append(pdfPaths, batchPath)

		for j := range batch {
			batch[j].Kardex.Movimientos = nil
		}
		startIndex += len(batch)
		shared.TriggerGC()
		shared.LogMemoryUsage(fmt.Sprintf("Batch %d/%d", i+1, len(batches)))
	}

	// PDF de totales globales (TOTAL GENERAL + RESUMEN + ANALISIS DE COSTOS)
	trk.Update(processKey, 75, "in-progress", "Generando totales globales...")
	totalsPath := filepath.Join(tempDir, fmt.Sprintf("%s_99_totales.pdf", filenameBase))
	if err := g.generateTotalsPDF(globalTotals, totalsPath); err != nil {
		return "", err
	}
	pdfPaths = append(pdfPaths, totalsPath)

	// Crear ZIP
	trk.Update(processKey, 80, "in-progress", "Comprimiendo archivos...")
	time.Sleep(100 * time.Millisecond)

	zipFilename := filenameBase + ".zip"
	zipPath := filepath.Join(folder, zipFilename)

	if err := shared.CreateZipFromFiles(pdfPaths, zipPath); err != nil {
		return "", err
	}

	trk.Update(processKey, 100, "completed", "Archivo ZIP generado")
	return zipFilename, nil
}

// =============================================================================
// AUXILIARES DE ESTRATEGIA
// =============================================================================

func (g *Generator) createOptimizedBatches(insumos []KardexInsumo) [][]KardexInsumo {
	batches := make([][]KardexInsumo, 0)
	currentBatch := make([]KardexInsumo, 0)
	currentMovementCount := 0

	for i := range insumos {
		movCount := len(insumos[i].Kardex.Movimientos)

		shouldCreateNewBatch := len(currentBatch) >= g.cfg.ItemsPerBatch ||
			(currentMovementCount+movCount > g.cfg.MaxMovementsPerBatch && len(currentBatch) > 0)

		if shouldCreateNewBatch {
			batches = append(batches, currentBatch)
			currentBatch = make([]KardexInsumo, 0)
			currentMovementCount = 0
		}

		currentBatch = append(currentBatch, insumos[i])
		currentMovementCount += movCount
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	log.Printf("[KardexPDF] Creados %d batches optimizados", len(batches))
	return batches
}

func (g *Generator) generateSummaryPDF(insumos []KardexInsumo, outputPath string) error {
	pdf := g.createNewPDF()
	g.addCoverPage(pdf)
	g.addProductSummaryPage(pdf, insumos)

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creando archivo: %v", err)
	}

	if err := pdf.Output(file); err != nil {
		file.Close()
		return fmt.Errorf("error escribiendo PDF: %v", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("error sincronizando archivo: %v", err)
	}

	return file.Close()
}

func (g *Generator) generateBatchOnlyPDF(batch []KardexInsumo, startIndex, totalInsumos int, outputPath string) error {
	pdf := g.createNewPDF()

	for i := range batch {
		g.addInsumoPage(pdf, &batch[i], startIndex+i+1, totalInsumos)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creando archivo: %v", err)
	}

	if err := pdf.Output(file); err != nil {
		file.Close()
		return fmt.Errorf("error escribiendo PDF: %v", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("error sincronizando archivo: %v", err)
	}

	return file.Close()
}

// generateTotalsPDF genera un PDF con solo la sección de totales globales (para ZIP).
func (g *Generator) generateTotalsPDF(gt GlobalTotals, outputPath string) error {
	pdf := g.createNewPDF()
	g.AddGlobalTotalsSection(pdf, gt)

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creando archivo: %v", err)
	}

	if err := pdf.Output(file); err != nil {
		file.Close()
		return fmt.Errorf("error escribiendo PDF: %v", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("error sincronizando archivo: %v", err)
	}

	return file.Close()
}

// =============================================================================
// PDF SETUP
// =============================================================================

func (g *Generator) createNewPDF() *gofpdf.Fpdf {
	pdf := gofpdf.New(g.orientation, "mm", "A4", "")
	pdf.SetMargins(7, 7.5, 7)
	pdf.SetAutoPageBreak(true, 5)
	pdf.SetFont("Arial", "", 8)
	pdf.SetCellMargin(0.1)
	pdf.AliasNbPages("{totalPages}")

	// Encabezado pegado al borde superior — sin espacio extra
	pdf.SetHeaderFunc(func() {
		pdf.SetY(0)
		pdf.SetFont("Arial", "B", 8)
		pdf.CellFormat(0, 4, "KARDEX VALORIZADO - "+shared.ToLatin(g.institution.RazonSocial), "", 1, "C", false, 0, "")
		pdf.SetFont("Arial", "", 7)
		pdf.CellFormat(0, 3.5, shared.ToLatin(g.periodo), "", 1, "C", false, 0, "")
	})

	// Pie de página pegado al borde inferior — solo espacio del texto
	pdf.SetFooterFunc(func() {
		pdf.SetY(-5)
		pdf.SetFont("Arial", "I", 7)
		pageStr := fmt.Sprintf("Pagina %d / {totalPages}", pdf.PageNo())
		pdf.CellFormat(0, 5, pageStr, "", 0, "C", false, 0, "")
	})

	return pdf
}

func (g *Generator) addCoverPage(pdf *gofpdf.Fpdf) {
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, "KARDEX VALORIZADO", "", 1, "C", false, 0, "")
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 6, shared.ToLatin(g.institution.RazonSocial), "", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 5, "RUC: "+g.institution.RUC, "", 1, "C", false, 0, "")

	if g.institution.Address != "" {
		pdf.SetFont("Arial", "", 9)
		pdf.CellFormat(0, 4, shared.ToLatin(g.institution.Address), "", 1, "C", false, 0, "")
	}

	pdf.Ln(5)
	pdf.SetFont("Arial", "B", 10)
	pdf.CellFormat(0, 5, shared.ToLatin(g.periodo), "", 1, "C", false, 0, "")
	pdf.Ln(3)
	pdf.SetFont("Arial", "", 9)
	pdf.CellFormat(0, 5, "Generado: "+g.timestamp, "", 1, "C", false, 0, "")
}
