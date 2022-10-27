package censusdb

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.vocdoni.io/dvote/censustree"
	"go.vocdoni.io/dvote/data/compressor"
	"go.vocdoni.io/dvote/db"
	"go.vocdoni.io/dvote/log"
	"go.vocdoni.io/proto/build/go/models"
)

const (
	censusDBprefix          = "cs_"
	censusDBreferencePrefix = "cr_"
)

// CensusRef is a reference to a census.
type CensusRef struct {
	tree       *censustree.Tree // must be private to avoid gob serialization
	AuthToken  *uuid.UUID
	CensusType int32
	Indexed    bool
	IsPublic   bool
}

// Tree returns the censustree.Tree object of the census reference.
func (cr *CensusRef) Tree() *censustree.Tree {
	return cr.tree
}

// SetTree sets the censustree.Tree object to the census reference.
func (cr *CensusRef) SetTree(tree *censustree.Tree) {
	cr.tree = tree
}

// CensusDump is a struct that contains the data of a census. It is used
// for import/export operations.
type CensusDump struct {
	Type     models.Census_Type `json:"type"`
	RootHash []byte             `json:"rootHash"`
	Data     []byte             `json:"data"`
	Indexed  bool               `json:"indexed"`
}

// CensusDB is a database of census trees.
type CensusDB struct {
	db        db.Database
	censusMap sync.Map
}

// NewCensusDB creates a new CensusDB object.
func NewCensusDB(db db.Database) *CensusDB {
	return &CensusDB{db: db}
}

// New creates a new census and adds it to the database.
func (c *CensusDB) New(censusID []byte, censusType models.Census_Type,
	indexed, public bool, authToken *uuid.UUID) (*CensusRef, error) {
	if c.Exists(censusID) {
		return nil, fmt.Errorf("census %x already exists", censusID)
	}
	tree, err := censustree.New(censustree.Options{Name: censusName(censusID), ParentDB: c.db,
		MaxLevels: 256, CensusType: censusType, IndexAsKeysCensus: indexed})
	if err != nil {
		return nil, err
	}
	ref, err := c.addCensusRefToDB(censusID, authToken, censusType, indexed, public)
	if err != nil {
		return nil, err
	}
	ref.tree = tree
	if public {
		ref.tree.Publish()
	}
	// add the tree in memory so we can quickly access to it afterwards
	c.censusMap.Store(string(censusID), *ref)
	return ref, nil
}

// Exists returns true if the censusID exists in the local database.
func (c *CensusDB) Exists(censusID []byte) bool {
	_, ok := c.censusMap.Load(string(censusID))
	return ok
}

// Load returns an already loaded census from memory or from the persistent kv database.
// Authentication is checked if authToken is not nil.
func (c *CensusDB) Load(censusID []byte, authToken *uuid.UUID) (*CensusRef, error) {
	// if the tree is in memory, just return it
	val, ok := c.censusMap.Load(string(censusID))
	if ok {
		ref, refOk := val.(CensusRef)
		if !refOk {
			panic("stored value is not of censusRef type")
		}
		if authToken != nil {
			if ref.AuthToken == nil {
				return nil, fmt.Errorf("census is locked")
			}
			if !bytes.Equal(authToken[:], ref.AuthToken[:]) {
				return nil, fmt.Errorf("wrong authentication token")
			}
		}
		return &ref, nil
	}

	// if not in memory, load census from DB
	ref, err := c.getCensusRefFromDB(censusID)
	if err != nil {
		return nil, err
	}
	// check authentication
	if authToken != nil {
		// if no token stored in the reference but the called provided a token, we don't allow
		if ref.AuthToken == nil {
			return nil, fmt.Errorf("census is locked")
		}
		if !bytes.Equal(authToken[:], ref.AuthToken[:]) {
			return nil, fmt.Errorf("wrong authentication token")
		}
	}
	ref.tree, err = censustree.New(censustree.Options{Name: censusName(censusID), ParentDB: c.db,
		MaxLevels: 256, CensusType: models.Census_Type(ref.CensusType)})
	if err != nil {
		return nil, err
	}
	if ref.IsPublic {
		ref.tree.Publish()
	}
	c.censusMap.Store(string(censusID), *ref)
	log.Debugf("loaded tree %x", censusID)
	return ref, nil
}

// Del removes a census from the database and memory.
func (c *CensusDB) Del(censusID []byte) error {
	wtx := c.db.WriteTx()
	defer wtx.Discard()
	if err := wtx.Delete(append([]byte(censusDBreferencePrefix), censusID...)); err != nil {
		return err
	}
	// the removal of the tree from the disk is done in a separate goroutine.
	// This is because the tree is locked and we don't want to block the operations,
	// and depending on the size of the tree, it can take a while to delete it.
	go func() {
		c.censusMap.Delete(string(censusID))
		_, err := censustree.DeleteCensusTreeFromDatabase(c.db, censusName(censusID))
		if err != nil {
			log.Warnf("error deleting census tree %x: %s", censusID, err)
		}
	}()
	return wtx.Commit()
}

// ImportAsPublic imports a census from a dump and makes it public.
func (c *CensusDB) ImportAsPublic(data []byte) error {
	cdata := CensusDump{}
	if err := json.Unmarshal(data, &cdata); err != nil {
		return err
	}
	if cdata.Data == nil || cdata.RootHash == nil {
		return fmt.Errorf("missing dump or root parameters")
	}
	log.Debugf("importing census %x of type %s", cdata.RootHash, cdata.Type.String())
	if c.Exists(cdata.RootHash) {
		return fmt.Errorf("could not import census %x, already exists", cdata.RootHash)
	}
	ref, err := c.New(cdata.RootHash, cdata.Type, cdata.Indexed, true, nil)
	if err != nil {
		return err
	}
	if err := ref.Tree().ImportDump(compressor.NewCompressor().DecompressBytes(cdata.Data)); err != nil {
		return err
	}
	root, err := ref.Tree().Root()
	if err != nil {
		return err
	}
	if !bytes.Equal(root, cdata.RootHash) {
		if err := c.Del(cdata.RootHash); err != nil {
			log.Warnf("could not delete census %x: %v", cdata.RootHash, err)
		}
		return fmt.Errorf("root hash does not match after importing dump")
	}

	return nil
}

// addCensusRefToDB adds a censusRef to the database.
func (c *CensusDB) addCensusRefToDB(censusID []byte, authToken *uuid.UUID,
	t models.Census_Type, indexed, public bool) (*CensusRef, error) {
	wtx := c.db.WriteTx()
	defer wtx.Discard()
	refData := bytes.Buffer{}
	enc := gob.NewEncoder(&refData)
	ref := &CensusRef{
		AuthToken:  authToken,
		CensusType: int32(t),
		Indexed:    indexed,
		IsPublic:   public,
	}
	if err := enc.Encode(ref); err != nil {
		return nil, err
	}
	if err := wtx.Set(append([]byte(censusDBreferencePrefix), censusID...),
		refData.Bytes()); err != nil {
		return nil, err
	}
	return ref, wtx.Commit()
}

// getCensusRefFromDB returns the censusRef from the database.
func (c *CensusDB) getCensusRefFromDB(censusID []byte) (*CensusRef, error) {
	b, err := c.db.ReadTx().Get(
		append(
			[]byte(censusDBreferencePrefix),
			censusID...,
		))
	if err != nil {
		if errors.Is(err, db.ErrKeyNotFound) {
			return nil, fmt.Errorf("census id not found in local storage")
		}
		return nil, err
	}
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	ref := CensusRef{}
	return &ref, dec.Decode(&ref)
}

// censusName returns the name of the census tree in the database.
func censusName(censusID []byte) string {
	return fmt.Sprintf("%s%x", censusDBprefix, censusID)
}
