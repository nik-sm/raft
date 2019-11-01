Blockchain Project Proposal

# Simplified Bitcoin Model
We will implement a simplified version of bitcoin, leaving aside the cryptographic signing of blocks and focusing on the gossiping of messages and construction of merkle trees and the actual blockchain.

## Nodes in the Network
The population of our network will consist of customer nodes and full miner nodes only. 
We will not implement "thin" miner nodes.
- Customer nodes will periodically attempt to generate transactions.
- Miner nodes will incorporate transactions into blocks, and attempt to mine blocks.

The population will be initialized with one fixed customer being given 10 coins.

On a random ticker, each customer with a positive wallet amount will generate a transaction sending a small random percentage of their wallet to one or more customer nodes.

See below for the structure of transactions.

Each node will begin with a `hostfile.json` listing the available customer and miner nodes in the following format:

```JSON
{
  "customers": [
    {"name": "c0", "id": 0}, 
    {"name": "c1", "id": 1},
    {"name": "c2", "id": 2}
  ]
  "miners": [
    {"name": "m0", "id": 0}
    {"name": "m1", "id": 1}
    {"name": "m2", "id": 2}
  ]
}
```

Nodes in the network can be resolved using DNS based on their `name` field, and addressed for transactions using their `id` field.
We will distinguish between these two fields because a real implementation (or a possible extension of the project) must distinguish between the public key information of a host and their network location.

We will also allow the special coinbase address: `{"name": "coinbase", "id": 999}`, to provide an `id` that can be used for transactions that generate coins (providing the initial coins for the network, as well as mining rewards).

## Miner Agreement
In the real network, miners are responsible for either storing the ledger, or consulting a set of trusted sources to determine the ledger.
Then, they can perform block mining starting from a valid point

## Honest customers
To begin with, we assume honest customer nodes, who only attempt to spend their coins once, and who only attempt to make transactions from their own wallet to another wallet.

This lets us begin by skipping all cryptographic signing and verification, focusing only on message, merkle tree construction, merkle tree verification.

To allow honest behavior, each customer must know the value of their own wallet when they try to generate a transaction.

As a possible test case, we can add some customers who attempt to double spend, and add some logic on another customer to validate the history of coins in the network.

## Transaction structure

## Proof of work
We will use the normal Bitcoin proof-of-work, but we will begin with no required `0` prefix.
This means any Nonce will succeed; to reduce thrashing in the network, we will control the rate of block production by having each miner wait for a random number of seconds between each successful hash computation.

## Messages

We will use the following message types, listed along with the fields they contain:

- NewTransaction
  - sourceID
  - destID
  - amount

- NewBlock
  - prevHash
  - timestamp
  - nonce
  - merkleTree

# Skeleton Types

```go
type sha256sum [32]byte
type walletID int
type cash int

type txnOutput struct {
  amt  cash
  dest walletID
}

type txn struct {
  from    walletID
  outputs []txnOutput
  fee     cash
  t       timestamp
}

type merkleTree struct {
  h      sha256sum
  isRoot bool
  isLeaf bool
  left   merkleTree // only present if !isLeaf
  right  merkleTree // only present if !isLeaf
  tx     txn        // only present if isLeaf
}

type block struct {
  prev block
  m     merkleTree
  t     timestamp
  txns  []txn
  nonce int
}

// Notice that we need to produce a predictable merkleTree from a given list of transactions
// This requires that we define a total ordering on transactions


// returns -1 if left is smaller, 0 if equal, and 1 if left is greater


// TODO - we need these lists to have same sort order, or just use maps
func (t []txnOutput) equals(other []txnOutput) bool {
  if len(t) != len(other) {
    return false
  }
  for i, item := range t {
    if item.amt != other[i].amt || item.dest != other[i].dest {
      return false
    }
  }
  return true
}

// TODO - basic unit tests
func (t timestamp) equals(other timestamp) bool {
  return false
}

func compare(t1 txn, t2 txn) int {
  if t1.from < t2.from {
    return -1
  } else if t1.from == t2.from {
    if t1.t < t2.t {
      return -1
    } else if t1.t == t2.t {
      return 0
    }
    return 1
  }
  return

}

```


# References and Useful Reading
- https://bitcoin.stackexchange.com/questions/69018/merkle-root-and-merkle-proofs
- http://www.iwar.org.uk/comsec/resources/cipher/sha256-384-512.pdf
- https://golang.org/pkg/crypto/sha256/
- https://en.bitcoin.it/wiki/Protocol_documentation#tx
- https://en.bitcoin.it/wiki/Transaction

# Questions
- In the setting with "full" and "thin" nodes:
  - If a thin requests for a merkle proof of a particular transaction being included in a particular block,
    the full node will send back a proof, consisting of 2 leaf nodes, and ~log(N) interior hashes, which together form a partial tree
    leading up to the merkle root that the thin node has.
  - What prevents the full node from working very quickly to falsify the tree and try to trick the thin node? The thin node has
    no way to verify the interior hashes that it receives, so they could easily be faked. Admittedly this may be a harder target,
    because we need to produce a fake interior hash (nonce) that produces the EXACT same value as some other interior hash,
    but we may have many places to try within the tree.
