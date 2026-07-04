package gotdx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultICFQSAddress    = "121.37.193.4:7615"
	DefaultICFQSHotAddress = "hot.icfqs.com:7615"
)

// ICFQSOption configures an ICFQSClient.
type ICFQSOption func(*ICFQSClient)

// ICFQSClient is a small HTTP client for ICFQS TQLEX endpoints.
type ICFQSClient struct {
	address    string
	httpClient *http.Client
}

// ICFQSCode identifies a market/code pair used by ICFQS HTTP APIs.
type ICFQSCode struct {
	Setcode string `json:"setcode"`
	Code    string `json:"code"`
}

// ICFQSTable is a table-shaped view of a TQLEX ResultSets item.
type ICFQSTable struct {
	Rows []map[string]any `json:"rows"`
}

// NewICFQS creates an ICFQS HTTP client.
func NewICFQS(opts ...ICFQSOption) *ICFQSClient {
	client := &ICFQSClient{
		address: DefaultICFQSAddress,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	return client
}

// NewICFQSHot creates an ICFQS HTTP client for hot.icfqs.com endpoints.
func NewICFQSHot(opts ...ICFQSOption) *ICFQSClient {
	client := NewICFQS(WithICFQSAddress(DefaultICFQSHotAddress))
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	return client
}

// WithICFQSAddress sets the ICFQS host:port or base URL.
func WithICFQSAddress(address string) ICFQSOption {
	return func(client *ICFQSClient) {
		if strings.TrimSpace(address) != "" {
			client.address = strings.TrimSpace(address)
		}
	}
}

// WithICFQSHTTPClient sets the HTTP client used by the ICFQS client.
func WithICFQSHTTPClient(httpClient *http.Client) ICFQSOption {
	return func(client *ICFQSClient) {
		if httpClient != nil {
			client.httpClient = httpClient
		}
	}
}

// WithICFQSTimeout sets the HTTP client timeout.
func WithICFQSTimeout(timeout time.Duration) ICFQSOption {
	return func(client *ICFQSClient) {
		if timeout > 0 {
			client.httpClient.Timeout = timeout
		}
	}
}

// PostTQL posts a TQLEX request with Params/oauth_zzfw and parses the response object.
func (client *ICFQSClient) PostTQL(ctx context.Context, entry string, params []any) (map[string]any, error) {
	body := map[string]any{
		"Params":     params,
		"oauth_zzfw": "1",
	}
	raw, err := client.post(ctx, entry, body)
	if err != nil {
		return nil, err
	}
	return parseICFQSTQLResponse(raw)
}

// PostJSON posts a raw JSON body to an ICFQS entry and decodes the standard JSON response.
func (client *ICFQSClient) PostJSON(ctx context.Context, entry string, body any) (map[string]any, error) {
	raw, err := client.post(ctx, entry, body)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ICFQSFormatTables converts ResultSets into rows keyed by ColName or ColDes.Name.
func ICFQSFormatTables(raw map[string]any) []ICFQSTable {
	resultSets, _ := raw["ResultSets"].([]any)
	tables := make([]ICFQSTable, 0, len(resultSets))
	for _, resultSet := range resultSets {
		rs, _ := resultSet.(map[string]any)
		columns := icfqsColumns(rs)
		content, _ := rs["Content"].([]any)
		table := ICFQSTable{Rows: make([]map[string]any, 0, len(content))}
		for _, rawRow := range content {
			values, _ := rawRow.([]any)
			row := make(map[string]any, len(columns))
			for i, column := range columns {
				if i < len(values) {
					row[column] = values[i]
				} else {
					row[column] = nil
				}
			}
			table.Rows = append(table.Rows, row)
		}
		tables = append(tables, table)
	}
	return tables
}

// ICFQSTopicListRaw queries the raw topic list endpoint.
func (client *ICFQSClient) ICFQSTopicListRaw(ctx context.Context, category string, setcode string, page int) (map[string]any, error) {
	if page <= 0 {
		page = 1
	}
	return client.PostTQL(ctx, "CWServ.ph_tdxdatacenter_zttz_zy", []any{"00601", category + "|" + setcode, page})
}

// ICFQSSearchTopicsRaw queries the raw topic search endpoint.
func (client *ICFQSClient) ICFQSSearchTopicsRaw(ctx context.Context, keyword string) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.ph_tdxdatacenter_zttz_zy", []any{"00102", keyword, "0"})
}

// ICFQSNewTopicsRaw queries the raw new-topic endpoint.
func (client *ICFQSClient) ICFQSNewTopicsRaw(ctx context.Context) (map[string]any, error) {
	return client.PostTQL(ctx, "DataAggregation.zttz_xzgn2", []any{"01001", "", 1})
}

// ICFQSHotTopicsRaw queries the raw hot-topic endpoint.
func (client *ICFQSClient) ICFQSHotTopicsRaw(ctx context.Context) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.ph_tdxdatacenter_zttz_zy", []any{"00302", "", 1})
}

// ICFQSEventsRaw queries the raw topic-events endpoint.
func (client *ICFQSClient) ICFQSEventsRaw(ctx context.Context) (map[string]any, error) {
	return client.PostTQL(ctx, "DataAggregation.zttz_jhqz", []any{"00401", "", 10})
}

// ICFQSTopTopicsRaw queries the raw leading-topic endpoint.
func (client *ICFQSClient) ICFQSTopTopicsRaw(ctx context.Context, topN int) (map[string]any, error) {
	if topN <= 0 {
		topN = 10
	}
	return client.PostTQL(ctx, "CWServ.ph_tdxdatacenter_zttz_zy", []any{"00101", "", topN})
}

// ICFQSTopicDetailRaw queries raw topic detail.
func (client *ICFQSClient) ICFQSTopicDetailRaw(ctx context.Context, code string, setcode string) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.ph_tdxdatacenter_zttz_xqy", []any{"00301", code, setcode})
}

// ICFQSTopicKLineRaw queries raw topic trend data.
func (client *ICFQSClient) ICFQSTopicKLineRaw(ctx context.Context, code string, setcode string) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.ph_tdxdatacenter_zttz_xqy_v2_qsid", []any{"00501", code, 3, "", setcode})
}

// ICFQSTopicStocksRaw queries raw topic member stocks.
func (client *ICFQSClient) ICFQSTopicStocksRaw(ctx context.Context, code string, setcode string, page int, size int) (map[string]any, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	return client.PostTQL(ctx, "CWServ.ph_tdxdatacenter_zttz_xggp", []any{"00901", code, setcode, 1, size, 0, page})
}

// ICFQSTopicQuotesRaw queries raw topic quote snapshots.
func (client *ICFQSClient) ICFQSTopicQuotesRaw(ctx context.Context, codes []ICFQSCode) (map[string]any, error) {
	codeValues, setcodeValues := splitICFQSCodes(codes)
	body := []map[string]any{{
		"ReqId":    "200800",
		"modname":  "module_misc.dll",
		"Code":     codeValues,
		"PageSize": fmt.Sprintf("%d", len(codeValues)),
		"Page":     "0",
		"Desc":     "0",
		"Setcode":  setcodeValues,
		"Sort":     "0",
	}}
	return client.PostJSON(ctx, "HQServ.hq_nlp", body)
}

// ICFQSTopicRotationRaw queries raw topic rotation data.
func (client *ICFQSClient) ICFQSTopicRotationRaw(ctx context.Context, dataNum int, dataType int, dataDate int, themeType string) (map[string]any, error) {
	if dataNum == 0 {
		dataNum = 1
	}
	if dataType == 0 {
		dataType = 1
	}
	if dataDate == 0 {
		dataDate = 2
	}
	if themeType == "" {
		themeType = "0"
	}
	body := []map[string]any{{
		"ReqId":     "200773",
		"modname":   "mod_copilot.dll",
		"dataDate":  fmt.Sprintf("%d", dataDate),
		"dataType":  fmt.Sprintf("%d", dataType),
		"dataNum":   fmt.Sprintf("%d", dataNum),
		"themeType": themeType,
	}}
	return client.PostJSON(ctx, "HQServ.hq_nlp_copilot", body)
}

// ICFQSQuotesBatchRaw queries raw stock quote snapshots.
func (client *ICFQSClient) ICFQSQuotesBatchRaw(ctx context.Context, codes []ICFQSCode, wantColumns []string) (map[string]any, error) {
	if len(wantColumns) == 0 {
		wantColumns = []string{"CLOSE", "NOW"}
	}
	codeValues, setcodeValues := splitICFQSCodes(codes)
	body := map[string]any{
		"Setcode": setcodeValues,
		"Head":    map[string]any{"Target": 0},
		"WantCol": wantColumns,
		"Code":    codeValues,
	}
	return client.PostJSON(ctx, "HQServ.PBCombHQ", body)
}

// ICFQSLHBDetailRaw queries raw stock LHB detail.
func (client *ICFQSClient) ICFQSLHBDetailRaw(ctx context.Context, symbol string, startDate string, endDate string) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.cfg_fx_yzlhb", []any{"yybxq", startDate, endDate, symbol, "", 0, 2000})
}

// ICFQSYYBDetailRaw queries raw brokerage LHB detail.
func (client *ICFQSClient) ICFQSYYBDetailRaw(ctx context.Context, yybName string, startDate string, endDate string) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.cfg_fx_yzlhb", []any{"tjyyb", startDate, endDate, "", yybName, 0, 2000})
}

// ICFQSYZDetailRaw queries raw active-capital detail.
func (client *ICFQSClient) ICFQSYZDetailRaw(ctx context.Context, code string, startDate string, endDate string) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.cfg_fx_yzlhb", []any{"yzxq", startDate, endDate, code, "", 0, 2000})
}

// ICFQSMRFPRaw queries raw daily review data by type.
func (client *ICFQSClient) ICFQSMRFPRaw(ctx context.Context, reviewType string, date string, limit int) (map[string]any, error) {
	if limit <= 0 {
		limit = 30
	}
	return client.PostTQL(ctx, "CWServ.cfg_tk_mrfp", []any{date, reviewType, "", 0, limit})
}

// ICFQSMRFPLatestDateRaw queries raw latest available daily review date.
func (client *ICFQSClient) ICFQSMRFPLatestDateRaw(ctx context.Context) (map[string]any, error) {
	return client.PostTQL(ctx, "CWServ.cfg_tk_mrfp", []any{"0", "rq", "", 0, 30})
}

func (client *ICFQSClient) post(ctx context.Context, entry string, body any) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.tqlURL(entry), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("icfqs %s returned status %d: %s", entry, resp.StatusCode, icfqsPreview(string(raw), 200))
	}
	return raw, nil
}

func (client *ICFQSClient) tqlURL(entry string) string {
	address := strings.TrimRight(client.address, "/")
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "http://" + address
	}
	return address + "/TQLEX?Entry=" + entry
}

func parseICFQSTQLResponse(raw []byte) (map[string]any, error) {
	start := bytes.IndexByte(raw, '{')
	if start < 0 {
		return nil, fmt.Errorf("cannot parse icfqs response: %s", icfqsPreview(string(raw), 200))
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var result map[string]any
				if err := json.Unmarshal(raw[start:i+1], &result); err != nil {
					return nil, err
				}
				return result, nil
			}
		}
	}
	return nil, fmt.Errorf("cannot parse icfqs response: %s", icfqsPreview(string(raw), 200))
}

func icfqsColumns(resultSet map[string]any) []string {
	if rawColumns, ok := resultSet["ColName"].([]any); ok && len(rawColumns) > 0 {
		columns := make([]string, 0, len(rawColumns))
		for _, rawColumn := range rawColumns {
			columns = append(columns, fmt.Sprint(rawColumn))
		}
		return columns
	}
	if rawColumnDefs, ok := resultSet["ColDes"].([]any); ok {
		columns := make([]string, 0, len(rawColumnDefs))
		for _, rawColumnDef := range rawColumnDefs {
			if columnDef, ok := rawColumnDef.(map[string]any); ok {
				columns = append(columns, fmt.Sprint(columnDef["Name"]))
			}
		}
		return columns
	}
	return nil
}

func splitICFQSCodes(codes []ICFQSCode) ([]string, []string) {
	codeValues := make([]string, 0, len(codes))
	setcodeValues := make([]string, 0, len(codes))
	for _, code := range codes {
		codeValues = append(codeValues, code.Code)
		setcodeValues = append(setcodeValues, code.Setcode)
	}
	return codeValues, setcodeValues
}

func icfqsPreview(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max]
}
