package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/database"
)

// Base entry columns that can be filtered directly on the entries table
var baseEntryColumns = map[string]bool{
	"path": true, "parent": true, "size": true, "kind": true,
	"ctime": true, "mtime": true, "last_scanned": true, "blocks": true,
}

var queryToolDef = mcp.NewTool("query",
	mcp.WithDescription("Unified search, filter, and aggregation across filesystem entries and attributes. Supports composable filters, sorting, pagination, and aggregation."),
	mcp.WithString("from",
		mcp.Description("Resource set name to query within, or omit for global search"),
	),
	mcp.WithObject("where",
		mcp.Description("Composable filters. Keys are attribute names, values are exact matches or operator objects (>, <, >=, <=, like, after, before)"),
	),
	mcp.WithArray("select",
		mcp.Description("Fields to return. Defaults to base attributes."),
	),
	mcp.WithString("aggregate",
		mcp.Description("Aggregation function: sum, count, avg, min, max"),
	),
	mcp.WithString("field",
		mcp.Description("Field for aggregation (e.g. size)"),
	),
	mcp.WithString("group_by",
		mcp.Description("Group aggregation results by this field"),
	),
	mcp.WithString("order_by",
		mcp.Description("Sort field. Prefix - for descending (e.g. -size)"),
	),
	mcp.WithNumber("limit",
		mcp.Description("Max results (default: 100)"),
	),
	mcp.WithString("cursor",
		mcp.Description("Opaque pagination cursor from a previous response"),
	),
)

func registerQueryTool(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(queryToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleQuery(ctx, request, db)
	})
}

func registerQueryToolMP(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(queryToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		return handleQuery(ctx, request, db)
	})
}

type queryArgs struct {
	From      *string                `json:"from,omitempty"`
	Where     map[string]interface{} `json:"where,omitempty"`
	Select    []string               `json:"select,omitempty"`
	Aggregate *string                `json:"aggregate,omitempty"`
	Field     *string                `json:"field,omitempty"`
	GroupBy   *string                `json:"group_by,omitempty"`
	OrderBy   *string                `json:"order_by,omitempty"`
	Limit     *int                   `json:"limit,omitempty"`
	Cursor    *string                `json:"cursor,omitempty"`
}

// cursorData encodes pagination state
type cursorData struct {
	Offset int `json:"o"`
}

func encodeCursor(offset int) string {
	data, _ := json.Marshal(cursorData{Offset: offset})
	return base64.StdEncoding.EncodeToString(data)
}

func decodeCursor(cursor string) (int, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor")
	}
	var cd cursorData
	if err := json.Unmarshal(data, &cd); err != nil {
		return 0, fmt.Errorf("invalid cursor")
	}
	return cd.Offset, nil
}

func handleQuery(ctx context.Context, request mcp.CallToolRequest, db *database.DiskDB) (*mcp.CallToolResult, error) {
	var args queryArgs
	if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	limit := 100
	if args.Limit != nil && *args.Limit > 0 {
		limit = *args.Limit
	}

	offset := 0
	if args.Cursor != nil && *args.Cursor != "" {
		var err error
		offset, err = decodeCursor(*args.Cursor)
		if err != nil {
			return mcp.NewToolResultError("Invalid cursor"), nil
		}
	}

	// Build WHERE clause
	whereClauses, whereParams, attrJoins, err := buildWhere(args.Where)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid where clause: %v", err)), nil
	}

	// Handle resource set filter
	fromJoin := ""
	if args.From != nil && *args.From != "" {
		fromJoin = `JOIN resource_set_entries rse ON rse.entry_path = e.path
			JOIN resource_sets rs ON rs.id = rse.set_id AND rs.name = ?`
		whereParams = append([]interface{}{*args.From}, whereParams...)
	}

	// Aggregate mode
	if args.Aggregate != nil && *args.Aggregate != "" {
		return handleAggregate(db, args, fromJoin, attrJoins, whereClauses, whereParams)
	}

	// Normal query mode
	return handleNormalQuery(db, args, fromJoin, attrJoins, whereClauses, whereParams, limit, offset)
}

func handleAggregate(db *database.DiskDB, args queryArgs, fromJoin, attrJoins, whereClauses string, whereParams []interface{}) (*mcp.CallToolResult, error) {
	aggFunc := strings.ToUpper(*args.Aggregate)
	if aggFunc != "SUM" && aggFunc != "COUNT" && aggFunc != "AVG" && aggFunc != "MIN" && aggFunc != "MAX" {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid aggregate function: %s", *args.Aggregate)), nil
	}

	field := "size"
	if args.Field != nil && *args.Field != "" {
		field = *args.Field
	}

	// Validate field is a base column for aggregation
	aggExpr := fmt.Sprintf("%s(e.%s)", aggFunc, field)
	if !baseEntryColumns[field] {
		return mcp.NewToolResultError(fmt.Sprintf("Cannot aggregate on non-base field %q", field)), nil
	}

	if args.GroupBy != nil && *args.GroupBy != "" {
		return handleGroupedAggregate(db, aggExpr, *args.GroupBy, fromJoin, attrJoins, whereClauses, whereParams)
	}

	query := fmt.Sprintf("SELECT %s FROM entries e %s %s %s", aggExpr, fromJoin, attrJoins, whereClauses)
	row := db.DB().QueryRow(query, whereParams...)
	var value float64
	if err := row.Scan(&value); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Aggregate query failed: %v", err)), nil
	}

	response := map[string]interface{}{"value": value}
	payload, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(payload)), nil
}

func handleGroupedAggregate(db *database.DiskDB, aggExpr, groupBy, fromJoin, attrJoins, whereClauses string, whereParams []interface{}) (*mcp.CallToolResult, error) {
	var groupExpr string
	if baseEntryColumns[groupBy] {
		groupExpr = "e." + groupBy
	} else {
		// Group by attribute value — need a join
		attrJoins += fmt.Sprintf(` LEFT JOIN attributes grp_attr ON grp_attr.entry_path = e.path AND grp_attr.key = '%s'`, groupBy)
		groupExpr = "grp_attr.value"
	}

	query := fmt.Sprintf("SELECT %s as group_key, %s as agg_value FROM entries e %s %s %s GROUP BY %s ORDER BY agg_value DESC",
		groupExpr, aggExpr, fromJoin, attrJoins, whereClauses, groupExpr)

	rows, err := db.DB().Query(query, whereParams...)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Grouped aggregate failed: %v", err)), nil
	}
	defer rows.Close()

	type groupResult struct {
		Group string  `json:"group"`
		Value float64 `json:"value"`
	}
	var groups []groupResult
	for rows.Next() {
		var g groupResult
		var groupKey *string
		if err := rows.Scan(&groupKey, &g.Value); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Scan error: %v", err)), nil
		}
		if groupKey != nil {
			g.Group = *groupKey
		} else {
			g.Group = "(null)"
		}
		groups = append(groups, g)
	}

	response := map[string]interface{}{"groups": groups}
	payload, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(payload)), nil
}

func handleNormalQuery(db *database.DiskDB, args queryArgs, fromJoin, attrJoins, whereClauses string, whereParams []interface{}, limit, offset int) (*mcp.CallToolResult, error) {
	// Build ORDER BY
	orderBy := "e.path ASC"
	if args.OrderBy != nil && *args.OrderBy != "" {
		ob := *args.OrderBy
		desc := false
		if strings.HasPrefix(ob, "-") {
			desc = true
			ob = ob[1:]
		}
		if baseEntryColumns[ob] {
			dir := "ASC"
			if desc {
				dir = "DESC"
			}
			orderBy = fmt.Sprintf("e.%s %s", ob, dir)
		}
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM entries e %s %s %s", fromJoin, attrJoins, whereClauses)
	var total int
	if err := db.DB().QueryRow(countQuery, whereParams...).Scan(&total); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Count query failed: %v", err)), nil
	}

	// Fetch rows
	query := fmt.Sprintf("SELECT e.path, e.parent, e.size, e.kind, e.ctime, e.mtime FROM entries e %s %s %s ORDER BY %s LIMIT ? OFFSET ?",
		fromJoin, attrJoins, whereClauses, orderBy)
	params := append(whereParams, limit, offset)

	rows, err := db.DB().Query(query, params...)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Query failed: %v", err)), nil
	}
	defer rows.Close()

	type entryResult struct {
		Path   string `json:"path"`
		Parent string `json:"parent,omitempty"`
		Size   int64  `json:"size"`
		Kind   string `json:"kind"`
		Ctime  int64  `json:"ctime"`
		Mtime  int64  `json:"mtime"`
	}

	var entries []entryResult
	for rows.Next() {
		var e entryResult
		var parent *string
		if err := rows.Scan(&e.Path, &parent, &e.Size, &e.Kind, &e.Ctime, &e.Mtime); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Scan error: %v", err)), nil
		}
		if parent != nil {
			e.Parent = *parent
		}
		entries = append(entries, e)
	}

	response := map[string]interface{}{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	}

	// Add cursor for next page if there are more results
	if offset+limit < total {
		response["next_cursor"] = encodeCursor(offset + limit)
	}

	payload, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(payload)), nil
}

// buildWhere converts the where map into SQL WHERE clauses
// Returns: whereClause string, params []interface{}, attrJoins string, error
func buildWhere(where map[string]interface{}) (string, []interface{}, string, error) {
	if len(where) == 0 {
		return "", nil, "", nil
	}

	var joinParams []interface{}
	var whereParams []interface{}
	var joinClauses []string
	var filterClauses []string
	attrIdx := 0

	for key, value := range where {
		if baseEntryColumns[key] {
			c, p, err := buildColumnFilter("e."+key, key, value)
			if err != nil {
				return "", nil, "", err
			}
			filterClauses = append(filterClauses, c)
			whereParams = append(whereParams, p...)
		} else {
			alias := fmt.Sprintf("a%d", attrIdx)
			attrIdx++
			joinClauses = append(joinClauses, fmt.Sprintf("JOIN attributes %s ON %s.entry_path = e.path AND %s.key = ?", alias, alias, alias))
			joinParams = append(joinParams, key)

			c, p, err := buildColumnFilter(alias+".value", key, value)
			if err != nil {
				return "", nil, "", err
			}
			filterClauses = append(filterClauses, c)
			whereParams = append(whereParams, p...)
		}
	}

	attrJoinStr := strings.Join(joinClauses, " ")
	whereStr := ""
	if len(filterClauses) > 0 {
		whereStr = "WHERE " + strings.Join(filterClauses, " AND ")
	}

	// Combine params: join params first (used in FROM), then where params
	allParams := append(joinParams, whereParams...)

	return whereStr, allParams, attrJoinStr, nil
}

func buildColumnFilter(colExpr, key string, value interface{}) (string, []interface{}, error) {
	switch v := value.(type) {
	case string:
		return colExpr + " = ?", []interface{}{v}, nil
	case float64:
		return colExpr + " = ?", []interface{}{int64(v)}, nil
	case bool:
		if v {
			return colExpr + " = 1", nil, nil
		}
		return colExpr + " = 0", nil, nil
	case map[string]interface{}:
		return buildOperatorFilter(colExpr, key, v)
	default:
		return colExpr + " = ?", []interface{}{fmt.Sprintf("%v", v)}, nil
	}
}

func buildOperatorFilter(colExpr, key string, ops map[string]interface{}) (string, []interface{}, error) {
	var clauses []string
	var params []interface{}

	for op, val := range ops {
		switch op {
		case ">":
			clauses = append(clauses, colExpr+" > ?")
			params = append(params, toNumeric(val))
		case "<":
			clauses = append(clauses, colExpr+" < ?")
			params = append(params, toNumeric(val))
		case ">=":
			clauses = append(clauses, colExpr+" >= ?")
			params = append(params, toNumeric(val))
		case "<=":
			clauses = append(clauses, colExpr+" <= ?")
			params = append(params, toNumeric(val))
		case "like":
			clauses = append(clauses, colExpr+" LIKE ?")
			params = append(params, val)
		case "not":
			clauses = append(clauses, colExpr+" != ?")
			params = append(params, val)
		case "after":
			ts, err := parseTimeValue(val)
			if err != nil {
				return "", nil, fmt.Errorf("invalid 'after' value for %s: %w", key, err)
			}
			clauses = append(clauses, colExpr+" > ?")
			params = append(params, ts)
		case "before":
			ts, err := parseTimeValue(val)
			if err != nil {
				return "", nil, fmt.Errorf("invalid 'before' value for %s: %w", key, err)
			}
			clauses = append(clauses, colExpr+" < ?")
			params = append(params, ts)
		default:
			return "", nil, fmt.Errorf("unknown operator %q", op)
		}
	}

	return strings.Join(clauses, " AND "), params, nil
}

func toNumeric(val interface{}) interface{} {
	switch v := val.(type) {
	case float64:
		return int64(v)
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
		return v
	default:
		return val
	}
}

func parseTimeValue(val interface{}) (int64, error) {
	switch v := val.(type) {
	case float64:
		return int64(v), nil
	case string:
		// Try parsing as date string
		formats := []string{"2006-01-02", "2006-01-02T15:04:05Z", time.RFC3339}
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				return t.Unix(), nil
			}
		}
		// Try as unix timestamp string
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n, nil
		}
		return 0, fmt.Errorf("cannot parse time value %q", v)
	default:
		return 0, fmt.Errorf("unsupported time value type")
	}
}
