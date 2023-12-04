package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/spf13/cobra"
	"golang.org/x/debug/internal/core"
	"golang.org/x/debug/internal/gocore"
	"golang.org/x/debug/internal/util"
)

type ObjRef struct {
	link string
	node *ObjNode
}

type ObjNode struct {
	addr core.Address
	name string
	size int64
	refs []*ObjRef
}

var (
	visitedNodes   = make(map[core.Address]bool)
	allNodes       = make(map[core.Address]*ObjNode)
	rootNodesMap   = make(map[core.Address]*ObjNode)
	rootObjNodes   = []*ObjNode{}
	globalVarNodes = []*ObjNode{}
	goroutineNodes = []*ObjNode{}
)

func genUniqueRefTree(nodes []*ObjNode) {
	// width first visit
	cnodes := []*ObjNode{}

	for _, node := range nodes {
		if _, ok := visitedNodes[node.addr]; !ok {
			panic("add children for not visited node")
			return
		}

		refs := allNodes[node.addr].refs // all refered child nodes
		for _, ref := range refs {
			if _, ok := visitedNodes[ref.node.addr]; ok {
				continue
			}
			newNode := nodeCopy(ref.node)
			newRef := &ObjRef{
				node: newNode,
				link: ref.link,
			}

			node.refs = append(node.refs, newRef)
			visitedNodes[newNode.addr] = true

			cnodes = append(cnodes, newNode)
		}
	}
	if len(cnodes) > 0 {
		genUniqueRefTree(cnodes)
	}
}

func (node *ObjNode) appendChild(cNode *ObjNode, link string) {
	ref := &ObjRef{
		link: link,
		node: cNode,
	}
	node.refs = append(node.refs, ref)
}

func findOrCreateObjNode(name string, addr core.Address, size int64) (*ObjNode, bool) {
	if node, ok := allNodes[addr]; ok {
		if node.size != size {
			fmt.Fprintf(os.Stderr, "same address: %v, old size: %v, new size: %v\n", addr, node.size, size)
		}
		if node.name != name && strings.HasPrefix(node.name, "unk") && !strings.HasPrefix(name, "unk") {
			node.name = name
		}
		return node, true
	}
	node := &ObjNode{
		name: name,
		addr: addr,
		size: size,
	}
	allNodes[addr] = node
	return node, false
}

func calcTreeSize(node *ObjNode) int64 {
	size := int64(0)
	for _, ref := range node.refs {
		size += calcTreeSize(ref.node)
	}
	node.size = size + node.size
	return node.size
}

func nodeCopy(node *ObjNode) *ObjNode {
	newNode := *node
	newNode.refs = []*ObjRef{}
	return &newNode
}

func addGlobalVarNodes(node *ObjNode) {
	n := nodeCopy(node)
	globalVarNodes = append(globalVarNodes, n)
	rootObjNodes = append(rootObjNodes, n)

	// mark visited for root node
	visitedNodes[n.addr] = true
}

func addGoroutines(node *ObjNode) {
	n := nodeCopy(node)
	goroutineNodes = append(goroutineNodes, node)
	rootObjNodes = append(rootObjNodes, n)

	// mark visited for root node
	visitedNodes[n.addr] = true
}

func runTopFunc(cmd *cobra.Command, args []string) {
	_, c, err := readCore()
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}

	objectType := args[0]

	topN, err := cmd.Flags().GetInt("top")
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}

	type object2Addr struct {
		Obj     gocore.Object
		Address core.Address
	}
	objMap := map[string][]*object2Addr{}
	c.ForEachObject(func(x gocore.Object) bool {
		objMap[typeName(c, x)] = append(objMap[typeName(c, x)],
			&object2Addr{
				Obj:     x,
				Address: c.Addr(x),
			})
		return true
	})

	type bucket struct {
		Count int
		Total int64
		Info  string
	}

	var buckets []*bucket

	info2Obj := map[string][]gocore.Object{}
	for _, oa := range objMap[objectType] {
		info := reachObjectsByAddress(c, oa.Address)
		split := strings.Split(info, "→")
		info2Obj[split[0]] = append(info2Obj[split[0]], oa.Obj)
	}
	for info, objs := range info2Obj {
		var total int64
		for _, obj := range objs {
			total = total + c.Size(obj)
		}
		buckets = append(buckets, &bucket{
			Count: len(objs),
			Total: total,
			Info:  info,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Total > buckets[j].Total
	})

	fmt.Printf("Object type : [%s], function stack info\n", objectType)
	// report only top N if requested
	if topN > 0 && len(buckets) > topN {
		buckets = buckets[:topN]
	}

	t := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintf(t, "%s\t%s\t %s\n", "Count", "Total", "Info")
	for _, e := range buckets {
		split := strings.Split(e.Info, "\n")
		output := ""
		for i, s := range split {
			if i == 0 {
				output = output + fmt.Sprintf("%d\t%s\t %s\n", e.Count, util.FormatBytes(e.Total), s)
			} else {
				output = output + fmt.Sprintf(" \t \t %s\n", s)
			}
		}
		_, _ = fmt.Fprintf(t, "%s", output)
	}
	_ = t.Flush()
}

func runObjref(cmd *cobra.Command, args []string) {
	minWidth, err := cmd.Flags().GetFloat64("minwidth")
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	printAddr, err := cmd.Flags().GetBool("printaddr")
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	_, c, err := readCore()
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}

	sumObjSize := int64(0)
	c.ForEachObject(func(x gocore.Object) bool {
		sumObjSize += c.Size(x)
		xNode, _ := findOrCreateObjNode(typeName(c, x), c.Addr(x), c.Size(x))
		c.ForEachPtr(x, func(i int64, y gocore.Object, j int64) bool {
			yNode, _ := findOrCreateObjNode(typeName(c, y), c.Addr(y), c.Size(y))
			xNode.appendChild(yNode, fieldName(c, x, i))
			return true
		})
		return true
	})
	_, _ = fmt.Fprintf(os.Stderr, "Sum object size %v\n", util.FormatBytes(sumObjSize))

	for _, r := range c.Globals() {
		// size = 0, since global variable is not from heap
		rNode, existing := findOrCreateObjNode(r.Name, r.Addr, 0)
		if !existing {
			// may have duplicated address from globals.
			addGlobalVarNodes(rNode)
		}

		c.ForEachRootPtr(r, func(i int64, y gocore.Object, j int64) bool {
			cNode, _ := findOrCreateObjNode(typeName(c, y), c.Addr(y), c.Size(y))
			rNode.appendChild(cNode, typeFieldName(r.Type, i))
			return true
		})
	}
	for _, g := range c.Goroutines() {
		gName := fmt.Sprintf("go%x", g.Addr())
		gNode, _ := findOrCreateObjNode(gName, g.Addr(), c.Size(gocore.Object(g.Addr())))
		addGoroutines(gNode)
	}

	// first, global variable
	genUniqueRefTree(globalVarNodes)
	// next, goroutines
	genUniqueRefTree(goroutineNodes)

	total := int64(0)
	for _, rNode := range rootObjNodes {
		total += calcTreeSize(rNode)
	}
	_, _ = fmt.Fprintf(os.Stderr, "Total size %v\n", util.FormatBytes(total))

	filename := args[0]
	// Dump object graph to output file.
	w, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	var path []string
	printedSize := int64(0)
	for _, rNode := range rootObjNodes {
		printedSize += printRefPath(w, path, total, rNode, minWidth, printAddr)
	}
	_, _ = fmt.Fprintf(os.Stderr, "Printed size: %v\n", util.FormatBytes(printedSize))
	w.Close()
	_, _ = fmt.Fprintf(os.Stderr, "Wrote the object reference to %q\n", filename)
}

func genRefPath(slice []string) string {
	// 1. reverse order.
	var reverse = make([]string, len(slice))

	for index, value := range slice {
		// 2. remove unprintable
		newValue := strings.Map(func(r rune) rune {
			if r == '+' || r == '?' {
				return '.'
			}
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, value)
		reverse[len(reverse)-1-index] = newValue
	}

	return strings.Join(reverse, "\n")
}

// return the printed size
func printRefPath(w *os.File, path []string, total int64, node *ObjNode, minWidth float64, printAddr bool) int64 {
	if float64(node.size)/float64(total) < minWidth/100 {
		return 0
	}
	printedSize := int64(0)
	if printAddr {
		path = append(path, fmt.Sprintf("%v 0x%x", node.name, node.addr))
	} else {
		path = append(path, node.name)
	}
	for _, ref := range node.refs {
		rPath := path
		if ref.link != "" {
			rPath = append(rPath, ref.link)
		}
		printedSize += printRefPath(w, rPath, total, ref.node, minWidth, printAddr)
	}
	if float64(node.size-printedSize)/float64(total) < minWidth/100 {
		return printedSize
	}
	ref := genRefPath(path)
	_, _ = fmt.Fprintf(w, "%v\n\t%d\n", ref, node.size-printedSize)

	return node.size
}
