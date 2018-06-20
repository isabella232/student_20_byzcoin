package eventlog

import (
	"bytes"
	"errors"
	"time"

	omniledger "github.com/dedis/cothority/omniledger/service"
	"github.com/dedis/cothority/skipchain"
	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/protobuf"
)

// ServiceName is the service name for the EventLog service.
var ServiceName = "EventLog"

var sid onet.ServiceID

func init() {
	var err error
	sid, err = onet.RegisterNewService(ServiceName, newService)
	if err != nil {
		log.Fatal(err)
	}
}

// Service is the EventLog service.
type Service struct {
	*onet.ServiceProcessor
	omni         *omniledger.Service
	bucketMaxAge time.Duration
}

const defaultBlockInterval = 5 * time.Second

// waitForBlock is for use in tests; it will sleep long enough to be sure that
// a block has been created.
func (s *Service) waitForBlock(scID skipchain.SkipBlockID) {
	dur, err := s.omni.LoadBlockInterval(scID)
	if err != nil {
		panic(err.Error())
	}
	time.Sleep(5 * dur)
}

// Init will create a new event log. Logs will be accepted
// from the signers mentioned in the request.
func (s *Service) Init(req *InitRequest) (*InitResponse, error) {
	cg := &omniledger.CreateGenesisBlock{
		Version:       omniledger.CurrentVersion,
		GenesisDarc:   req.Owner,
		Roster:        req.Roster,
		BlockInterval: req.BlockInterval,
	}
	cgr, err := s.omni.CreateGenesisBlock(cg)
	if err != nil {
		return nil, err
	}

	return &InitResponse{
		ID: cgr.Skipblock.Hash,
	}, nil
}

// Log will create a new event log entry.
func (s *Service) Log(req *LogRequest) (*LogResponse, error) {
	req2 := &omniledger.AddTxRequest{
		Version:     omniledger.CurrentVersion,
		SkipchainID: req.SkipchainID,
		Transaction: req.Transaction,
	}
	_, err := s.omni.AddTransaction(req2)
	if err != nil {
		return nil, err
	}
	return &LogResponse{}, nil
}

// GetEvent asks omniledger for a stored event.
func (s *Service) GetEvent(req *GetEventRequest) (*GetEventResponse, error) {
	req2 := omniledger.GetProof{
		Version: omniledger.CurrentVersion,
		Key:     req.Key,
		ID:      req.SkipchainID,
	}
	reply, err := s.omni.GetProof(&req2)
	if err != nil {
		return nil, err
	}
	if !reply.Proof.InclusionProof.Match() {
		return nil, errors.New("not an inclusion proof")
	}
	k, vs, err := reply.Proof.KeyValue()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(k, req2.Key) {
		return nil, errors.New("wrong key")
	}
	if len(vs) < 2 {
		return nil, errors.New("not enough values")
	}
	e := Event{}
	err = protobuf.Decode(vs[0], &e)
	if err != nil {
		return nil, err
	}
	return &GetEventResponse{
		Event: e,
	}, nil
}

// This should be a const, but we want to be able to hack it from tests.
var searchMax = 10000

// Search will search the event log for matching entries.
func (s *Service) Search(req *SearchRequest) (*SearchResponse, error) {
	if req.ID.IsNull() {
		return nil, errors.New("skipchain ID required")
	}

	if req.To == 0 {
		req.To = time.Now().UnixNano()
	}

	v := s.omni.GetCollectionView(req.ID)
	el := &eventLog{ID: theEventLog.Slice(), v: v}

	id, b, err := el.getLatestBucket()
	if err == errIndexMissing {
		// There are no events yet on this chain, so return no results.
		return &SearchResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	// bEnd is normally updated from the last bucket's start. For the latest
	// bucket, bEnd is now.
	bEnd := time.Now().UnixNano()

	// Walk backwards in the bucket chain through 2 zones: first where the
	// bucket covers time that is not in our search range, and then where the buckets
	// do cover the search range. When we see a bucket that ends before our search
	// range, we can stop walking buckets.
	var buckets []*bucket
	var bids [][]byte
	for {
		if req.From > bEnd {
			// This bucket is before the search range, so we are done walking back the bucket chain.
			break
		}

		if req.To < b.Start {
			// This bucket is after the search range, so we do not add it to buckets, but
			// we keep walking up the chain.
		} else {
			buckets = append(buckets, b)
			bids = append(bids, id)
		}

		if b.isFirst() {
			break
		}
		bEnd = b.Start
		id = b.Prev
		b, err = el.getBucketByID(id)
		if err != nil {
			// This indicates that the event log data structure is wrong, so
			// we cannot claim to correctly search it. Give up instead.
			log.Errorf("expected event log bucket id %x not found: %v", b.Prev, err)
			return nil, err
		}
	}

	reply := &SearchResponse{}

	// Process the time buckets from earliest to latest so that
	// if we truncate, it is the latest events that are not returned,
	// so that they can set req.From = resp.Events[len(resp.Events)-1].When.
filter:
	for i := len(buckets) - 1; i >= 0; i-- {
		b := buckets[i]
		for _, e := range b.EventRefs {
			ev, err := getEventByID(v, e)
			if err != nil {
				log.Errorf("bucket %x points to event %x, but the event was not found: %v", bids[i], e, err)
				return nil, err
			}

			if req.From <= ev.When && ev.When < req.To {
				if req.Topic == "" || req.Topic == ev.Topic {
					reply.Events = append(reply.Events, *ev)
					if len(reply.Events) >= searchMax {
						reply.Truncated = true
						break filter
					}
				}
			}
		}
	}

	return reply, nil
}

const contractName = "eventlog"

func (s *Service) decodeAndCheckEvent(coll omniledger.CollectionView, eventBuf []byte) (*Event, error) {
	// Check the timestamp of the event: it should never be in the future,
	// and it should not be more than 5 seconds in the past. (Why 5?
	// Because a #of blocks limit is too fragile when using fast blocks for tests.)
	event := &Event{}
	err := protobuf.Decode(eventBuf, event)
	if err != nil {
		return nil, err
	}
	when := time.Unix(0, event.When)
	now := time.Now()
	if when.Before(now.Add(-5 * time.Second)) {
		return nil, errors.New("event timestamp too long ago")
	}
	if when.After(now) {
		return nil, errors.New("event timestamp is in the future")
	}
	return event, nil
}

// contractFunction is the function that runs to process a transaction of
// type "eventlog"
func (s *Service) contractFunction(v omniledger.CollectionView, tx omniledger.Instruction, c []omniledger.Coin) ([]omniledger.StateChange, []omniledger.Coin, error) {
	if tx.Delete != nil {
		return nil, nil, errors.New("delete tx not allowed")
	}
	if tx.Invoke != nil {
		return nil, nil, errors.New("invoke tx not allowed")
	}
	if tx.Spawn == nil {
		return nil, nil, errors.New("expected a spawn tx")
	}

	// This is not strictly required, because since we know we are
	// a spawn, we know the contract comes directly from
	// tx.Spawn.ContractID.
	cid, _, err := tx.GetContractState(v)
	if err != nil {
		return nil, nil, err
	}

	// All the state changes, at every step, go in here.
	scs := []omniledger.StateChange{}

	eventBuf := tx.Spawn.Args.Search("event")
	if eventBuf == nil {
		return nil, nil, errors.New("expected a named argument of \"event\"")
	}

	event, err := s.decodeAndCheckEvent(v, eventBuf)
	if err != nil {
		return nil, nil, err
	}

	scs = append(scs, omniledger.NewStateChange(omniledger.Create, tx.ObjectID, cid, eventBuf))

	// Walk from latest bucket back towards beginning looking for the right bucket.
	//
	// If you don't find a bucket with b.Start <= ev.When,
	// create a new bucket, put in the event, set the start, emit the bucket,
	// update prev in the bucket before (and also possibly the index key).
	//
	// If you find an existing latest bucket, and b.Start is more than X seconds
	// ago, make a new bucket anyway.
	//
	// If you find the right bucket, add the event and emit the updated bucket.
	// For now: buckets are allowed to grow as big as needed (but the previous
	// rule prevents buckets from getting too big by timing them out).

	el := &eventLog{ID: theEventLog.Slice(), v: v}
	bID, b, err := el.getLatestBucket()
	if err != nil && err != errIndexMissing {
		return nil, nil, err
	}
	isHead := true

	for b != nil && !b.isFirst() {
		if b.Start <= event.When {
			break
		}
		bID = b.Prev
		b, err = el.getBucketByID(bID)
		if err != nil {
			return nil, nil, err
		}
		isHead = false
	}

	// Make a new head bucket if:
	//   No latest bucket: b == nil
	//     or
	//   Found a bucket, and it is head, and is too old
	if b == nil || isHead && time.Duration(event.When-b.Start) > s.bucketMaxAge {
		newBid := tx.DeriveID("bucket")

		if b == nil {
			// Special case: The first bucket for an eventlog
			// needs a catch-all bucket before it, in case later
			// events come in.
			catchID := tx.DeriveID("catch-all")
			newb := &bucket{
				Start:     0,
				Prev:      nil,
				EventRefs: nil,
			}
			buf, err := protobuf.Encode(newb)
			if err != nil {
				return nil, nil, err
			}
			scs = append(scs, omniledger.NewStateChange(omniledger.Create, catchID, cid, buf))
			bID = catchID.Slice()
		}

		// This new bucket will start with this event.
		newb := &bucket{
			Start:     event.When,
			Prev:      bID,
			EventRefs: [][]byte{tx.ObjectID.Slice()},
		}
		buf, err := protobuf.Encode(newb)
		if err != nil {
			return nil, nil, err
		}
		scs = append(scs, omniledger.NewStateChange(omniledger.Create, newBid, cid, buf))

		// Create/Update the pointer to the latest bucket.
		action := omniledger.Update
		if b == nil {
			action = omniledger.Create
		}
		scs = append(scs, omniledger.NewStateChange(action, theEventLog, cid, newBid.Slice()))
	} else {
		// Otherwise just add into whatever bucket we found, no matter how
		// many are already there. (Splitting buckets is hard and not important to us.)
		b.EventRefs = append(b.EventRefs, tx.ObjectID.Slice())
		bucketBuf, err := protobuf.Encode(b)
		if err != nil {
			return nil, nil, err
		}
		scs = append(scs,
			omniledger.StateChange{
				StateAction: omniledger.Update,
				ObjectID:    bID,
				ContractID:  []byte(contractName),
				Value:       bucketBuf,
			})
	}

	return scs, nil, nil
}

// newService receives the context that holds information about the node it's
// running on. Saving and loading can be done using the context. The data will
// be stored in memory for tests and simulations, and on disk for real
// deployments.
func newService(c *onet.Context) (onet.Service, error) {
	s := &Service{
		ServiceProcessor: onet.NewServiceProcessor(c),
		omni:             c.Service(omniledger.ServiceName).(*omniledger.Service),
		// Set a relatively low time for bucketMaxAge: during peak message arrival
		// this will pretect the buckets from getting too big. During low message
		// arrival (< 1 per 5 sec) it does not create extra buckets, because time
		// periods with no events do not need buckets created for them.
		bucketMaxAge: 5 * time.Second,
	}
	if err := s.RegisterHandlers(s.Init, s.Log, s.GetEvent, s.Search); err != nil {
		log.ErrFatal(err, "Couldn't register messages")
	}

	omniledger.RegisterContract(s, contractName, s.contractFunction)
	return s, nil
}

func getEventByID(coll omniledger.CollectionView, objID []byte) (*Event, error) {
	r, err := coll.Get(objID).Record()
	if err != nil {
		return nil, err
	}
	v, err := r.Values()
	if err != nil {
		return nil, err
	}
	newval, ok := v[0].([]byte)
	if !ok {
		return nil, errors.New("invalid value")
	}
	var e Event
	if err := protobuf.Decode(newval, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
