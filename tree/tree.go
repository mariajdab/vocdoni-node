// Package tree provides the functions for creating and managing an iden3 merkletree
package tree

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"

	common3 "github.com/iden3/go-iden3-core/common"
	"github.com/iden3/go-iden3-core/core/claims"

	"github.com/iden3/go-iden3-core/db"
	"github.com/iden3/go-iden3-core/merkletree"
	"golang.org/x/text/unicode/norm"
)

type Tree struct {
	StorageDir string
	Tree       *merkletree.MerkleTree
	DbStorage  *db.LevelDbStorage
}

const MaxClaimSize = 120

func (t *Tree) MaxClaimSize() int {
	return MaxClaimSize
}

func (t *Tree) Init(namespace string) error {
	if len(t.StorageDir) < 1 {
		if len(namespace) < 1 {
			return errors.New("namespace not valid")
		}
		home, err := os.UserHomeDir()
		if err == nil {
			t.StorageDir = home + "/.dvote/census"
		} else {
			t.StorageDir = "./dvoteTree"
		}
	}
	mtdb, err := db.NewLevelDbStorage(t.StorageDir+"/"+namespace, false)
	if err != nil {
		return err
	}
	mt, err := merkletree.NewMerkleTree(mtdb, 140)
	if err != nil {
		return err
	}
	t.DbStorage = mtdb
	t.Tree = mt
	return nil
}

func (t *Tree) Close() {
	t.Tree.Storage().Close()
}

func (t *Tree) Claim(data []byte) (*claims.ClaimBasic, error) {
	if len(data) > MaxClaimSize {
		return nil, errors.New("claim data too large")
	}
	if len(data) < MaxClaimSize {
		doPadding(&data)
	}
	return getClaimFromData(data), nil
}

func getClaimFromData(data []byte) *claims.ClaimBasic {
	var indexSlot [112]byte
	var dataSlot [MaxClaimSize]byte
	copy(indexSlot[:], data[:400/8])
	copy(dataSlot[:], data[:MaxClaimSize])
	return claims.NewClaimBasic(indexSlot, dataSlot)
}

func doPadding(data *[]byte) {
	for i := len(*data); i < MaxClaimSize; i++ {
		*data = append(*data, byte('\x00'))
	}
}

func (t *Tree) AddClaim(data []byte) error {
	if len(data) < MaxClaimSize {
		doPadding(&data)
	}
	e := getClaimFromData(data)
	return t.Tree.AddEntry(e.Entry())
}

func (t *Tree) GenProof(data []byte) (string, error) {
	if len(data) < MaxClaimSize {
		doPadding(&data)
	}
	e, err := t.Claim(data)
	if err != nil {
		return "", err
	}
	hash, err := e.Entry().HIndex()
	if err != nil {
		return "", err
	}
	mp, err := t.Tree.GenerateProof(hash, nil)
	if err != nil {
		return "", err
	}
	if !mp.Existence {
		return "", nil
	}
	mpHex := common3.HexEncode(mp.Bytes())
	return mpHex, nil
}

// CheckProof standalone function for checking a merkle proof
func CheckProof(root, mpHex string, data []byte) (bool, error) {
	mpBytes, err := common3.HexDecode(mpHex)
	if err != nil {
		return false, err
	}
	mp, err := merkletree.NewProofFromBytes(mpBytes)
	if err != nil {
		return false, err
	}
	rootHash, err := stringToHash(root)
	if err != nil {
		return false, err
	}
	if len(data) < MaxClaimSize {
		doPadding(&data)
	}
	e := getClaimFromData(data)
	hvalue, err := e.Entry().HValue()
	if err != nil {
		return false, err
	}
	hindex, err := e.Entry().HIndex()
	if err != nil {
		return false, err
	}
	return merkletree.VerifyProof(&rootHash, mp,
		hindex, hvalue), nil
}

func (t *Tree) CheckProof(data []byte, mpHex string) (bool, error) {
	return CheckProof(t.Root(), mpHex, data)
}

func (t *Tree) Root() string {
	return common3.HexEncode(t.Tree.RootKey().Bytes())
}

func (t *Tree) Index(data []byte) (string, error) {
	e, err := t.Claim(data)
	if err != nil {
		return "", err
	}
	hash, err := e.Entry().HIndex()
	if err != nil {
		return "", err
	}
	index, err := t.Tree.GetDataByIndex(hash)
	return index.String(), err
}

func stringToHash(hash string) (merkletree.Hash, error) {
	var rootHash merkletree.Hash
	rootBytes, err := common3.HexDecode(hash)
	if err != nil {
		return rootHash, err
	}
	copy(rootHash[:32], rootBytes)
	return rootHash, err
}

func (t *Tree) Dump(root string) (claims []string, err error) {
	var rootHash merkletree.Hash
	if len(root) > 0 {
		rootHash, err = stringToHash(root)
		if err != nil {
			return
		}
	}
	claims, err = t.Tree.DumpClaims(&rootHash)
	return
}

// Size returns the number of leaf nodes on the merkle tree
func (t *Tree) Size(root string) (int64, error) {
	var err error
	var rootHash merkletree.Hash
	var size int64
	if len(root) > 0 {
		rootHash, err = stringToHash(root)
		if err != nil {
			return size, err
		}
	}
	err = t.Tree.Walk(&rootHash, func(n *merkletree.Node) {
		if n.Type == merkletree.NodeTypeLeaf {
			size++
		}
	})
	return size, err
}

func (t *Tree) DumpPlain(root string, responseBase64 bool) ([]string, error) {
	var response []string
	var err error
	var rootHash merkletree.Hash
	if len(root) > 0 {
		rootHash, err = stringToHash(root)
		if err != nil {
			return response, err
		}
	}
	err = t.Tree.Walk(&rootHash, func(n *merkletree.Node) {
		if n.Type == merkletree.NodeTypeLeaf {
			var data [MaxClaimSize + 8]byte
			i := 0
			for _, e := range n.Entry.Value() {
				copy(data[i:i+len(e)], e[:])
				i = i + len(e)
			}
			dataClean := bytes.Replace(data[:], []byte("\x00"), nil, -1)
			if responseBase64 {
				datab64 := base64.StdEncoding.EncodeToString(dataClean)
				response = append(response, datab64)
			} else {
				response = append(response, string(norm.NFC.Bytes(dataClean)))
			}
		}
	})
	return response, err
}

func (t *Tree) ImportDump(claims []string) error {
	return t.Tree.ImportDumpedClaims(claims)
}

func (t *Tree) Snapshot(root string) (*Tree, error) {
	rootHash := new(merkletree.Hash)
	snapshotTree := new(Tree)
	var err error
	if len(root) > 0 {
		*rootHash, err = stringToHash(root)
		if err != nil || rootHash == nil {
			return snapshotTree, err
		}
	}
	mt, err := t.Tree.Snapshot(rootHash)
	snapshotTree.Tree = mt
	return snapshotTree, err
}

func (t *Tree) HashExist(hash string) (bool, error) {
	h, err := stringToHash(hash)
	if err != nil {
		return false, err
	}
	n, err := t.Tree.GetNode(&h)
	if err != nil || n == nil {
		return false, nil
	}
	return true, nil
}
