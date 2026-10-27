package kardex

import "kardex-pdf-service/internal/shared"

// =============================================================================
// DOMAIN TYPES
// =============================================================================

// Institution representa la información de la institución.
type Institution struct {
	ID              shared.FlexibleString `json:"id,omitempty"`
	IDInstitucion   shared.FlexibleString `json:"idinstitucion,omitempty"`
	RUC             string                `json:"ruc"`
	NombreComercial string                `json:"nombreComercial"`
	RazonSocial     string                `json:"razonSocial"`
	Address         string                `json:"address,omitempty"`
	ImgUrl          string                `json:"imgUrl,omitempty"`
}

// KardexMovimiento representa un movimiento individual del kardex.
type KardexMovimiento struct {
	IDKardexInsumo  int                   `json:"idkardex_insumo"`
	KardexEmpresaID int                   `json:"kardex_empresa_idkardex_empresa"`
	IDEmpresa       shared.FlexibleString `json:"idempresa"`
	IDInsumo        shared.FlexibleString `json:"idinsumo"`
	IDDetalleInsumo shared.FlexibleString `json:"iddetalle_insumo"`
	Codigo          string                `json:"codigo"`
	Fuente          string                `json:"fuente"`
	Asunto          string                `json:"asunto"` // "salida" | "compra" | "proforma"
	SerieDocumento  string                `json:"serie_documento"`
	NumeroDocumento string                `json:"numero_documento"`
	TipoDocumento   string                `json:"tipo_documento"`
	TipoOperacion   string                `json:"tipo_operacion"`
	FechaKardex     string                `json:"fecha_kardex"`
	HoraKardex      string                `json:"hora_kardex"`
	EstadoKardex    string                `json:"estado_kardex"` // "0" | "1"
	EstadoDocumento string                `json:"estado_documento"` // "" | "00".."06" (04 rechazado / 03 observado / 05 anulado)
	IDDetalle       int                   `json:"iddetalle"`
	CantidadDetalle float64               `json:"cantidad_detalle"`
	PrecioDetalle   float64               `json:"precio_detalle"`
	ParcialDetalle  float64               `json:"parcial_detalle"`
	NombreInsumo    string                `json:"nombre_insumo"`
	CodigoInsumo    string                `json:"codigo_insumo"`
	Sistema         string                `json:"sistema"`
	DB              string                `json:"db"`
	StockReservado  float64               `json:"stock_reservado"`

	// Campos de entrada
	CantidadIngreso *float64 `json:"cantidad_ingreso,omitempty"`
	PrecioIngreso   *float64 `json:"precio_ingreso,omitempty"`
	ParcialIngreso  *float64 `json:"parcial_ingreso,omitempty"`

	// Campos de salida
	CantidadSalida *float64 `json:"cantidad_salida,omitempty"`
	PrecioSalida   *float64 `json:"precio_salida,omitempty"`
	ParcialSalida  *float64 `json:"parcial_salida,omitempty"`

	// Campos finales
	CantidadFinal *float64 `json:"cantidad_final,omitempty"`
	PrecioFinal   *float64 `json:"precio_final,omitempty"`
	CostoFinal    *float64 `json:"costo_final,omitempty"`

	IsFinal bool `json:"isFinal,omitempty"`

	// Series/lotes del movimiento (el render Go aún no los dibuja; el Excel sí)
	Series []KardexSerie `json:"series,omitempty"`
	Lotes  []KardexLote  `json:"lotes,omitempty"`
}

// KardexSerie serie de producto asociada a un movimiento.
type KardexSerie struct {
	Code string `json:"code"`
}

// KardexLote lote asociado a un movimiento.
type KardexLote struct {
	Lote        string  `json:"lote"`
	Vencimiento string  `json:"vencimiento"`
	Cantidad    float64 `json:"cantidad"`
}

// KardexInitial representa el saldo inicial.
type KardexInitial struct {
	Cantidad       float64 `json:"cantidad"`
	Precio         float64 `json:"precio"`
	Parcial        float64 `json:"parcial"`
	StockReservado float64 `json:"stock_reservado"`
}

// KardexFinish representa el saldo final.
type KardexFinish struct {
	Cantidad       float64 `json:"cantidad"`
	Precio         float64 `json:"precio"`
	Parcial        float64 `json:"parcial"`
	StockReservado float64 `json:"stock_reservado"`
}

// KardexData contiene los movimientos y saldos.
type KardexData struct {
	Movimientos []KardexMovimiento `json:"movimientos"`
	Initial     *KardexInitial     `json:"initial"`
	Finish      *KardexFinish      `json:"finish"`
}

// InsumoDetail contiene los detalles del insumo.
type InsumoDetail struct {
	ID     shared.FlexibleString `json:"id,omitempty"`
	Insumo *InsumoInfo           `json:"insumo"`
}

// Category información de categoría.
type Category struct {
	ID           shared.FlexibleString `json:"id"`
	Denomination string                `json:"denomination"`
}

// Brand información de marca.
type Brand struct {
	ID           shared.FlexibleString `json:"id"`
	Denomination string                `json:"denomination"`
}

// InsumoInfo información del producto/insumo.
type InsumoInfo struct {
	ID           shared.FlexibleString `json:"id"`
	Denomination string                `json:"denomination"`
	Barcode      string                `json:"barcode"`
	UnitSimbol   string                `json:"unitSimbol,omitempty"`
	UnidadMedida string                `json:"unidadMedida,omitempty"`
	State        string                `json:"state,omitempty"`
	Contabilidad string                `json:"contabilidad,omitempty"`
	Category     *Category             `json:"category,omitempty"`
	Subcategory  *Category             `json:"subcategory,omitempty"`
	Brand        *Brand                `json:"brand,omitempty"`
}

// KardexInsumo representa un insumo con su kardex completo.
type KardexInsumo struct {
	InsumoDetail InsumoDetail `json:"insumoDetail"`
	Kardex       KardexData   `json:"kardex"`
}

// =============================================================================
// REQUEST/RESPONSE
// =============================================================================

// GeneratePDFRequest estructura para solicitud de generación.
type GeneratePDFRequest struct {
	JSONFilePath    string `json:"jsonFilePath"`
	OutputDirectory string `json:"outputDirectory"`
	OutputFilename  string `json:"outputFilename"`
	ReportType      string `json:"reportType,omitempty"`    // "completo" | "resumen_saldos"
	Orientation     string `json:"orientation,omitempty"`   // "L" (landscape, default) | "P" (portrait)
	TableStyle      string `json:"tableStyle,omitempty"`    // "simple" | "formal" | "contador"
}

// KardexParams parámetros adicionales del kardex.
type KardexParams struct {
	IDInstitucion string `json:"idinstitucion"`
	DateStart     string `json:"dateStart"`
	DateEnd       string `json:"dateEnd"`
	DateType      string `json:"dateType"`
	Valores       string `json:"valores,omitempty"` // "e" | "d"
	Idioma        string `json:"idioma,omitempty"`  // "es" | "en"
	// Flags de inclusión
	IsSerie       bool `json:"isSerie,omitempty"`
	DetalleSeries bool `json:"detalleSeries,omitempty"` // Modo Detallado: una fila por serie
	ConLotes bool `json:"conLotes,omitempty"`
}

// KardexJSONData estructura de los datos en el archivo JSON.
type KardexJSONData struct {
	Institution Institution    `json:"institution"`
	Insumos     []KardexInsumo `json:"insumos"`
	Params      KardexParams   `json:"params"`
}

// GeneratePDFResponse respuesta de la generación.
type GeneratePDFResponse struct {
	Success        bool   `json:"success"`
	Filename       string `json:"filename"`
	FilePath       string `json:"filePath"`
	TotalPages     int    `json:"totalPages"`
	TotalInsumos   int    `json:"totalInsumos"`
	TotalMovements int    `json:"totalMovements"`
	Strategy       string `json:"strategy"` // "single" | "merge" | "zip"
	Duration       string `json:"duration"`
	MemoryUsedMB   int    `json:"memoryUsedMb"`
	Error          string `json:"error,omitempty"`
}

// Metrics métricas calculadas del dataset.
type Metrics struct {
	TotalItems     int
	TotalMovements int
}

// =============================================================================
// GENERATE-ALL (3 formatos en 1 llamada)
// =============================================================================

// GenerateAllResult resultado individual de un formato dentro de generate-all.
type GenerateAllResult struct {
	ReportType  string `json:"reportType"`
	Filename    string `json:"filename"`
	TotalPages  int    `json:"totalPages"`
	Error       string `json:"error,omitempty"`
}

// GenerateAllResponse respuesta del endpoint /generate-all.
type GenerateAllResponse struct {
	Success        bool                `json:"success"`
	Results        []GenerateAllResult `json:"results"`
	TotalInsumos   int                 `json:"totalInsumos"`
	TotalMovements int                 `json:"totalMovements"`
	Duration       string              `json:"duration"`
	MemoryUsedMB   int                 `json:"memoryUsedMb"`
	Error          string              `json:"error,omitempty"`
}
