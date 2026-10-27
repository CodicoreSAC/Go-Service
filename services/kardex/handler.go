package kardex

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"kardex-pdf-service/internal/config"
	"kardex-pdf-service/internal/shared"
	"kardex-pdf-service/internal/tracker"
)

// RegisterRoutes registra las rutas del servicio kardex en un router group.
func RegisterRoutes(rg *gin.RouterGroup, cfg *config.AppConfig, trk *tracker.ProcessTracker) {
	rg.POST("/generate", generatePDFHandler(cfg, trk))
	rg.POST("/generate-all", generateAllHandler(cfg, trk))
	rg.GET("/progress", progressHandler(trk))
}

func generatePDFHandler(cfg *config.AppConfig, trk *tracker.ProcessTracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		var req GeneratePDFRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{
				Success: false,
				Error:   fmt.Sprintf("JSON inválido: %v", err),
			})
			return
		}

		if req.JSONFilePath == "" {
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{Success: false, Error: "No se proporcionó la ruta del archivo JSON (jsonFilePath)"})
			return
		}
		if req.OutputDirectory == "" {
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{Success: false, Error: "No se proporcionó el directorio de salida (outputDirectory)"})
			return
		}
		if req.OutputFilename == "" {
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{Success: false, Error: "No se proporcionó el nombre del archivo (outputFilename)"})
			return
		}

		// Leer archivo JSON
		jsonData, err := os.ReadFile(req.JSONFilePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{Success: false, Error: fmt.Sprintf("Error leyendo archivo JSON: %v", err)})
			return
		}

		// Parsear datos
		var kardexData KardexJSONData
		if err := json.Unmarshal(jsonData, &kardexData); err != nil {
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{Success: false, Error: fmt.Sprintf("Error parseando datos JSON: %v", err)})
			return
		}

		if len(kardexData.Insumos) == 0 {
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{Success: false, Error: "No se encontraron insumos en el archivo JSON"})
			return
		}

		// Clave de proceso única
		institutionID := kardexData.Institution.IDInstitucion.String()
		if institutionID == "" {
			institutionID = kardexData.Institution.ID.String()
		}
		processKey := fmt.Sprintf("pdf-%s-%d", institutionID, time.Now().UnixNano())

		shared.LogMemoryUsage("🚀 Inicio del proceso")

		// Formatear periodo
		periodo := shared.FormatPeriodo(kardexData.Params.DateType, kardexData.Params.DateStart)

		// Crear generador
		gen := NewGenerator(kardexData.Institution, periodo, cfg, req.Orientation, req.TableStyle, kardexData.Params.DetalleSeries)

		// Calcular métricas
		metrics := gen.CalculateMetrics(kardexData.Insumos)

		// Determinar estrategia
		var strategy string
		if metrics.TotalMovements <= cfg.SingleFileThreshold {
			strategy = "single"
		} else {
			strategy = "merge"
		}

		// Generar PDF
		filenameBase := req.OutputFilename
		if len(filenameBase) > 4 && filenameBase[len(filenameBase)-4:] == ".pdf" {
			filenameBase = filenameBase[:len(filenameBase)-4]
		}

		reportType := req.ReportType
		if reportType == "" {
			reportType = "completo"
		}

		var filename string
		if reportType == "resumen_saldos" {
			filename, err = gen.GenerateSummary(kardexData.Insumos, filenameBase, req.OutputDirectory, processKey, trk)
		} else if reportType == "sunat_13_1" {
			filename, err = gen.GenerateSUNAT13(kardexData.Insumos, filenameBase, req.OutputDirectory, processKey, trk)
		} else if reportType == "anual" {
			// anual: pendiente de implementación
			c.JSON(http.StatusBadRequest, GeneratePDFResponse{Success: false, Error: "Reporte anual aún no implementado"})
			return
		} else {
			// completo (default)
			filename, err = gen.Generate(kardexData.Insumos, filenameBase, req.OutputDirectory, processKey, trk)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, GeneratePDFResponse{Success: false, Error: fmt.Sprintf("Error generando PDF: %v", err)})
			return
		}

		filePath := fmt.Sprintf("%s/%s", req.OutputDirectory, filename)

		log.Printf("[KardexPDF] 📄 PDF generado en: %s", filePath)

		// Liberar memoria
		kardexData.Insumos = nil
		jsonData = nil
		shared.TriggerGC()

		duration := time.Since(startTime)
		shared.LogMemoryUsage("🏁 Proceso finalizado")

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		c.JSON(http.StatusOK, GeneratePDFResponse{
			Success:        true,
			Filename:       filename,
			FilePath:       filePath,
			TotalPages:     gen.TotalPages(),
			TotalInsumos:   metrics.TotalItems,
			TotalMovements: metrics.TotalMovements,
			Strategy:       strategy,
			Duration:       duration.String(),
			MemoryUsedMB:   int(m.Alloc / 1024 / 1024),
		})
	}
}

func generateAllHandler(cfg *config.AppConfig, trk *tracker.ProcessTracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		var req GeneratePDFRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, GenerateAllResponse{
				Success: false,
				Error:   fmt.Sprintf("JSON inválido: %v", err),
			})
			return
		}

		if req.JSONFilePath == "" {
			c.JSON(http.StatusBadRequest, GenerateAllResponse{Success: false, Error: "No se proporcionó la ruta del archivo JSON (jsonFilePath)"})
			return
		}
		if req.OutputDirectory == "" {
			c.JSON(http.StatusBadRequest, GenerateAllResponse{Success: false, Error: "No se proporcionó el directorio de salida (outputDirectory)"})
			return
		}
		if req.OutputFilename == "" {
			c.JSON(http.StatusBadRequest, GenerateAllResponse{Success: false, Error: "No se proporcionó el nombre del archivo (outputFilename)"})
			return
		}

		// Leer archivo JSON (1 sola vez)
		jsonData, err := os.ReadFile(req.JSONFilePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, GenerateAllResponse{Success: false, Error: fmt.Sprintf("Error leyendo archivo JSON: %v", err)})
			return
		}

		var kardexData KardexJSONData
		if err := json.Unmarshal(jsonData, &kardexData); err != nil {
			c.JSON(http.StatusBadRequest, GenerateAllResponse{Success: false, Error: fmt.Sprintf("Error parseando datos JSON: %v", err)})
			return
		}

		if len(kardexData.Insumos) == 0 {
			c.JSON(http.StatusBadRequest, GenerateAllResponse{Success: false, Error: "No se encontraron insumos en el archivo JSON"})
			return
		}

		institutionID := kardexData.Institution.IDInstitucion.String()
		if institutionID == "" {
			institutionID = kardexData.Institution.ID.String()
		}
		processKey := fmt.Sprintf("pdf-all-%s-%d", institutionID, time.Now().UnixNano())

		shared.LogMemoryUsage("🚀 [GenerateAll] Inicio del proceso")

		periodo := shared.FormatPeriodo(kardexData.Params.DateType, kardexData.Params.DateStart)

		filenameBase := req.OutputFilename
		if len(filenameBase) > 4 && filenameBase[len(filenameBase)-4:] == ".pdf" {
			filenameBase = filenameBase[:len(filenameBase)-4]
		}

		// Generar los 3 formatos
		results := GenerateAll(kardexData.Insumos, kardexData.Institution, periodo, filenameBase, req.OutputDirectory, processKey, trk, cfg, req.Orientation, req.TableStyle, kardexData.Params.DetalleSeries)

		for _, r := range results {
			if r.Filename != "" {
				log.Printf("[KardexPDF-All] 📄 %s → %s/%s", r.ReportType, req.OutputDirectory, r.Filename)
			}
		}

		metrics := NewGenerator(kardexData.Institution, periodo, cfg, "L", req.TableStyle, kardexData.Params.DetalleSeries).CalculateMetrics(kardexData.Insumos)

		// Liberar memoria
		kardexData.Insumos = nil
		jsonData = nil
		shared.TriggerGC()

		duration := time.Since(startTime)
		shared.LogMemoryUsage("🏁 [GenerateAll] Proceso finalizado")

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		// Verificar si todos tuvieron éxito
		allSuccess := true
		for _, r := range results {
			if r.Error != "" {
				allSuccess = false
				break
			}
		}

		c.JSON(http.StatusOK, GenerateAllResponse{
			Success:        allSuccess,
			Results:        results,
			TotalInsumos:   metrics.TotalItems,
			TotalMovements: metrics.TotalMovements,
			Duration:       duration.String(),
			MemoryUsedMB:   int(m.Alloc / 1024 / 1024),
		})
	}
}

func progressHandler(trk *tracker.ProcessTracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		processKey := c.Query("key")
		if processKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key requerida"})
			return
		}

		progress := trk.Get(processKey)
		if progress == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "proceso no encontrado"})
			return
		}

		c.JSON(http.StatusOK, progress)
	}
}
