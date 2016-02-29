package sda_test

import (
	"strconv"
	"testing"

	"github.com/dedis/cothority/lib/cliutils"
	"github.com/dedis/cothority/lib/dbg"
	"github.com/dedis/cothority/lib/network"
	"github.com/dedis/cothority/lib/sda"
	"github.com/dedis/crypto/abstract"
	"github.com/satori/go.uuid"
	"net"
)

var tSuite = network.Suite
var prefix = "localhost:"

// test the ID generation
func TestTreeId(t *testing.T) {
	names := genLocalhostPeerNames(3, 2000)
	idsList := genEntityList(tSuite, names)
	// Generate two example topology
	tree := idsList.GenerateBinaryTree()
	/*
			TODO: re-calculate the uuid
		root, _ := ExampleGenerateTreeFromEntityList(idsList)
		tree := sda.Tree{IdList: idsList, Root: root}
		var h bytes.Buffer
		h.Write(idsList.Id().Bytes())
		h.Write(root.Id().Bytes())
		genId := uuid.NewV5(uuid.NamespaceURL, h.String())
		if !uuid.Equal(genId, tree.Id()) {
				t.Fatal("Id generated is wrong")
			}
	*/
	if len(tree.Id.String()) != 36 {
		t.Fatal("Id generated is wrong")
	}
}

// Test if topology correctly handles the "virtual" connections in the topology
func TestTreeConnectedTo(t *testing.T) {
	names := genLocalhostPeerNames(3, 2000)
	peerList := genEntityList(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	// Generate the network
	if !tree.Root.IsConnectedTo(peerList.List[1]) {
		t.Fatal("Root should be connected to child (localhost:2001)")
	}
	if !tree.Root.IsConnectedTo(peerList.List[2]) {
		t.Fatal("Root should be connected to child (localhost:2002)")
	}
}

// Test initialisation of new peer-list
func TestEntityListNew(t *testing.T) {
	adresses := []string{"localhost:1010", "localhost:1012"}
	pl := genEntityList(tSuite, adresses)
	if len(pl.List) != 2 {
		t.Fatalf("Expected two peers in PeerList. Instead got %d", len(pl.List))
	}
	if pl.Id == uuid.Nil {
		t.Fatal("PeerList without ID is not allowed")
	}
	if len(pl.Id.String()) != 36 {
		t.Fatal("PeerList ID does not seem to be a UUID.")
	}
}

// Test initialisation of new peer-list from config-file
func TestInitPeerListFromConfigFile(t *testing.T) {
	names := genLocalhostPeerNames(3, 2000)
	idsList := genEntityList(tSuite, names)
	// write it
	WriteTomlConfig(idsList.Toml(tSuite), "identities.toml", "testdata")
	// decode it
	var decoded sda.EntityListToml
	if err := ReadTomlConfig(&decoded, "identities.toml", "testdata"); err != nil {
		t.Fatal("COuld not read from file the entityList")
	}
	decodedList := decoded.EntityList(tSuite)
	if len(decodedList.List) != 3 {
		t.Fatalf("Expected two identities in EntityList. Instead got %d", len(decodedList.List))
	}
	if decodedList.Id == uuid.Nil {
		t.Fatal("PeerList without ID is not allowed")
	}
	if len(decodedList.Id.String()) != 36 {
		t.Fatal("PeerList ID does not seem to be a UUID hash.")
	}
}

// Test initialisation of new random tree from a peer-list

// Test initialisation of new graph from config-file using a peer-list
// XXX again this test might be obsolete/does more harm then it is useful:
// It forces every field to be exported/made public
// and we want to get away from config files (or not?)

// Test initialisation of new graph when one peer is represented more than
// once

// Test access to tree:
// - parent
func TestTreeParent(t *testing.T) {
	names := genLocalhostPeerNames(3, 2000)
	peerList := genEntityList(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	child := tree.Root.Children[0]
	if child.Parent.Id != tree.Root.Id {
		t.Fatal("Parent of child of root is not the root...")
	}
}

// - children
func TestTreeChildren(t *testing.T) {
	names := genLocalhostPeerNames(2, 2000)
	peerList := genEntityList(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	child := tree.Root.Children[0]
	if child.Entity.Id != peerList.List[1].Id {
		t.Fatal("Parent of child of root is not the root...")
	}
}

// Test marshal/unmarshaling of trees
func TestUnMarshalTree(t *testing.T) {
	dbg.TestOutput(testing.Verbose(), 4)
	names := genLocalhostPeerNames(10, 2000)
	peerList := genEntityList(tSuite, names)
	// Generate two example topology
	tree := peerList.GenerateBinaryTree()
	tree_binary, err := tree.Marshal()

	if err != nil {
		t.Fatal("Error while marshaling:", err)
	}
	if len(tree_binary) == 0 {
		t.Fatal("Marshaled tree is empty")
	}

	tree2, err := sda.NewTreeFromMarshal(tree_binary, peerList)
	if err != nil {
		t.Fatal("Error while unmarshaling:", err)
	}
	if !tree.Equal(tree2) {
		dbg.Lvl3(tree, "\n", tree2)
		t.Fatal("Tree and Tree2 are not identical")
	}
}

func TestGetNode(t *testing.T) {
	tree, _ := genLocalTree(10, 2000)
	for _, tn := range tree.ListNodes() {
		node := tree.GetTreeNode(tn.Id)
		if node == nil {
			t.Fatal("Didn't find treeNode with id", tn.Id)
		}
	}
}

func TestBinaryTree(t *testing.T) {
	tree, _ := genLocalTree(7, 2000)
	root := tree.Root
	if len(root.Children) != 2 {
		t.Fatal("Not two children from root")
	}
	if len(root.Children[0].Children) != 2 {
		t.Fatal("Not two children from first child")
	}
	if len(root.Children[1].Children) != 2 {
		t.Fatal("Not two children from second child")
	}
	if !tree.IsBinary(root) {
		t.Fatal("Tree should be binary")
	}
}

func TestNaryTree(t *testing.T) {
	dbg.TestOutput(testing.Verbose(), 4)
	names := genLocalhostPeerNames(13, 2000)
	peerList := genEntityList(tSuite, names)
	tree := peerList.GenerateNaryTree(3)
	root := tree.Root
	if len(root.Children) != 3 {
		t.Fatal("Not three children from root")
	}
	if len(root.Children[0].Children) != 3 {
		t.Fatal("Not three children from first child")
	}
	if len(root.Children[1].Children) != 3 {
		t.Fatal("Not three children from second child")
	}
	if len(root.Children[2].Children) != 3 {
		t.Fatal("Not three children from third child")
	}
	if !tree.IsNary(root, 3) {
		t.Fatal("Tree should be 3-ary")
	}

	dbg.TestOutput(testing.Verbose(), 4)
	names = genLocalhostPeerNames(14, 2000)
	peerList = genEntityList(tSuite, names)
	tree = peerList.GenerateNaryTree(3)
	root = tree.Root
	if len(root.Children) != 3 {
		t.Fatal("Not three children from root")
	}
	if len(root.Children[0].Children) != 3 {
		t.Fatal("Not three children from first child")
	}
	if len(root.Children[1].Children) != 3 {
		t.Fatal("Not three children from second child")
	}
	if len(root.Children[2].Children) != 3 {
		t.Fatal("Not three children from third child")
	}
}

func TestBigNaryTree(t *testing.T) {
	dbg.TestOutput(testing.Verbose(), 4)
	names := genLocalDiffPeerNames(3, 2000)
	peerList := genEntityList(tSuite, names)
	tree := peerList.GenerateBigNaryTree(3, 13)
	root := tree.Root
	dbg.Lvl2(tree.Dump())
	if !tree.IsNary(root, 3) {
		t.Fatal("Tree should be 3-ary")
	}
	for _, child := range root.Children {
		if child.Entity.Id == root.Entity.Id {
			t.Fatal("Child should not have same identity as parent")
		}
		for _, c := range child.Children {
			if c.Entity.Id == child.Entity.Id {
				t.Fatal("Child should not have same identity as parent")
			}
		}
	}
}

func TestTreeIsColored(t *testing.T) {
	dbg.TestOutput(testing.Verbose(), 4)
	names := []string{"local1:1000", "local1:1001", "local2:1000", "local2:1001"}
	peerList := genEntityList(tSuite, names)
	tree := peerList.GenerateBigNaryTree(3, 13)
	root := tree.Root
	rootHost, _, _ := net.SplitHostPort(root.Entity.Addresses[0])
	for _, child := range root.Children {
		childHost, _, _ := net.SplitHostPort(child.Entity.Addresses[0])
		if rootHost == childHost {
			t.Fatal("Child", childHost, "is the same as root", rootHost)
		}
	}
}

func TestBinaryTrees(t *testing.T) {
	tree, _ := genLocalTree(1, 2000)
	if !tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 1 children should be binary")
	}
	tree, _ = genLocalTree(2, 2000)
	if tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 2 children should NOT be binary")
	}
	tree, _ = genLocalTree(3, 2000)
	if !tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 3 children should be binary")
	}
	tree, _ = genLocalTree(4, 2000)
	if tree.IsBinary(tree.Root) {
		t.Fatal("Tree with 4 children should be binary")
	}
}

// - public keys
// - corner-case: accessing parent/children with multiple instances of the same peer
// in the graph

// genLocalhostPeerNames will generate n localhost names with port indices starting from p
func genLocalhostPeerNames(n, p int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = prefix + strconv.Itoa(p+i)
	}
	return names
}

// genLocalDiffPeerNames will generate n local0..n-1 names with port indices starting from p
func genLocalDiffPeerNames(n, p int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = "local" + strconv.Itoa(i) + ":2000"
	}
	return names
}

// genEntityList generates a EntityList out of names
func genEntityList(suite abstract.Suite, names []string) *sda.EntityList {
	var ids []*network.Entity
	for _, n := range names {
		kp := cliutils.KeyPair(suite)
		ids = append(ids, network.NewEntity(kp.Public, n))
	}
	return sda.NewEntityList(ids)
}

func genLocalTree(count, port int) (*sda.Tree, *sda.EntityList) {
	names := genLocalhostPeerNames(count, port)
	peerList := genEntityList(tSuite, names)
	tree := peerList.GenerateBinaryTree()
	return tree, peerList
}
