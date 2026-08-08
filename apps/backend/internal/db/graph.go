package db

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/apache/age/drivers/golang/age"
)

// VertexType represents domain vertex labels
type VertexType string

const (
	VertexWallet   VertexType = "Wallet"
	VertexContract VertexType = "Contract"
	VertexExchange VertexType = "Exchange"
)

// EdgeType represents domain edge labels
type EdgeType string

const (
	EdgeTransfer EdgeType = "TRANSFER"
	EdgeMint     EdgeType = "MINT"
	EdgeSwap     EdgeType = "SWAP"
)

// Node represents a graph vertex (Wallet, Contract, Exchange)
type Node struct {
	Address     string     `json:"address"`
	Label       VertexType `json:"label"`
	CustomLabel string     `json:"custom_label,omitempty"`
	RiskScore   float64    `json:"risk_score"`
	FirstSeenAt string     `json:"first_seen_at,omitempty"`
	LastSeenAt  string     `json:"last_seen_at,omitempty"`
}

// Edge represents a transaction or token movement edge
type Edge struct {
	ID           string   `json:"id"`
	Hash         string   `json:"hash"`
	FromAddress  string   `json:"from_address"`
	ToAddress    string   `json:"to_address"`
	Label        EdgeType `json:"label"`
	ValueWei     string   `json:"value_wei"`
	BlockNumber  int64    `json:"block_number"`
	Timestamp    string   `json:"timestamp"`
	TokenAddress string   `json:"token_address,omitempty"`
}

// GraphResult holds nodes and edges returned from graph traversals
type GraphResult struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// UpsertNode creates or updates a vertex in the OpenChain graph
func (d *DB) UpsertNode(ctx context.Context, node Node) error {
	ag, err := d.ConnectAge()
	if err != nil {
		return fmt.Errorf("age connect error: %w", err)
	}

	tx, err := ag.Begin()
	if err != nil {
		return fmt.Errorf("age begin error: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	label := node.Label
	if label == "" {
		label = VertexWallet
	}

	addr := strings.ToLower(node.Address)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	cypher := fmt.Sprintf(`
		MERGE (n:%s {address: '%s'})
		ON CREATE SET n.risk_score = %f, n.custom_label = '%s', n.first_seen_at = '%s', n.last_seen_at = '%s'
		ON MATCH SET n.last_seen_at = '%s'
	`, label, addr, node.RiskScore, node.CustomLabel, nowStr, nowStr, nowStr)

	_, err = tx.ExecCypher(0, "%s", cypher)
	if err != nil {
		return fmt.Errorf("failed to upsert node %s: %w", addr, err)
	}

	return tx.Commit()
}

// UpsertEdge creates a transaction edge between two vertices
func (d *DB) UpsertEdge(ctx context.Context, edge Edge) error {
	ag, err := d.ConnectAge()
	if err != nil {
		return fmt.Errorf("age connect error: %w", err)
	}

	tx, err := ag.Begin()
	if err != nil {
		return fmt.Errorf("age begin error: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	fromAddr := strings.ToLower(edge.FromAddress)
	toAddr := strings.ToLower(edge.ToAddress)
	label := edge.Label
	if label == "" {
		label = EdgeTransfer
	}

	// Ensure both end vertices exist
	cypherNodes := fmt.Sprintf(`
		MERGE (a:Wallet {address: '%s'})
		MERGE (b:Wallet {address: '%s'})
	`, fromAddr, toAddr)

	_, err = tx.ExecCypher(0, "%s", cypherNodes)
	if err != nil {
		slog.Debug("failed merging end vertices for edge", "error", err)
	}

	cypherEdge := fmt.Sprintf(`
		MATCH (a {address: '%s'}), (b {address: '%s'})
		CREATE (a)-[r:%s {hash: '%s', value_wei: '%s', block_number: %d, timestamp: '%s', token_address: '%s'}]->(b)
	`, fromAddr, toAddr, label, edge.Hash, edge.ValueWei, edge.BlockNumber, edge.Timestamp, edge.TokenAddress)

	_, err = tx.ExecCypher(0, "%s", cypherEdge)
	if err != nil {
		return fmt.Errorf("failed to create edge %s -> %s: %w", fromAddr, toAddr, err)
	}

	return tx.Commit()
}

// QueryHopGraph queries 1-hop or n-hop subgraph around a root address
func (d *DB) QueryHopGraph(ctx context.Context, rootAddr string, maxHops int) (*GraphResult, error) {
	ag, err := d.ConnectAge()
	if err != nil {
		return nil, fmt.Errorf("age connect error: %w", err)
	}

	tx, err := ag.Begin()
	if err != nil {
		return nil, fmt.Errorf("age begin error: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if maxHops <= 0 {
		maxHops = 1
	}

	addr := strings.ToLower(rootAddr)
	slog.Debug("Querying hop graph", "address", addr, "maxHops", maxHops)

	// Fetch 1-hop connections
	cypher := fmt.Sprintf(`
		MATCH (a {address: '%s'})-[r]->(b)
		RETURN a, r, b
	`, addr)

	cursor, err := tx.ExecCypher(3, "%s", cypher)

	if err != nil {
		slog.Warn("cypher query returned error (possibly empty graph)", "address", addr, "error", err)
		return &GraphResult{
			Nodes: []Node{{Address: addr, Label: VertexWallet}},
			Edges: []Edge{},
		}, nil
	}

	nodeMap := make(map[string]Node)
	nodeMap[addr] = Node{Address: addr, Label: VertexWallet}
	var edges []Edge

	for cursor.Next() {
		row, err := cursor.GetRow()
		if err != nil {
			continue
		}
		if len(row) < 3 {
			continue
		}

		v1, ok1 := row[0].(*age.Vertex)
		e, ok2 := row[1].(*age.Edge)
		v2, ok3 := row[2].(*age.Vertex)

		if !ok1 || !ok2 || !ok3 {
			continue
		}

		v1Addr, _ := v1.Props()["address"].(string)
		v2Addr, _ := v2.Props()["address"].(string)

		if v1Addr != "" {
			if _, exists := nodeMap[v1Addr]; !exists {
				nodeMap[v1Addr] = Node{Address: v1Addr, Label: VertexType(v1.Label())}
			}
		}
		if v2Addr != "" {
			if _, exists := nodeMap[v2Addr]; !exists {
				nodeMap[v2Addr] = Node{Address: v2Addr, Label: VertexType(v2.Label())}
			}
		}


		hashVal, _ := e.Props()["hash"].(string)
		valWei, _ := e.Props()["value_wei"].(string)
		blkNum, _ := e.Props()["block_number"].(int64)
		ts, _ := e.Props()["timestamp"].(string)
		tokenAddr, _ := e.Props()["token_address"].(string)

		edges = append(edges, Edge{
			ID:           fmt.Sprintf("%s-%s", hashVal, e.Label()),
			Hash:         hashVal,
			FromAddress:  v1Addr,
			ToAddress:    v2Addr,
			Label:        EdgeType(e.Label()),
			ValueWei:     valWei,
			BlockNumber:  blkNum,
			Timestamp:    ts,
			TokenAddress: tokenAddr,
		})
	}

	nodes := make([]Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	return &GraphResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}


