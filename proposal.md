Project Proposal

# Simplified Bitcoin Model
We will implement a simplified version of bitcoin as follows.

## Nodes in the Network
The population of our network consists of customer nodes and miner nodes.
- Customer nodes will periodically attempt to generate transactions.
- Miner nodes will perform standard mining. 
  These miner nodes will correspond to "full" nodes in a typical bitcoin setting, and we will not implement "thin" nodes.

The population will be initialized with each one customer being given 10 coins.

After random intervals of time, each customer with a positive wallet amount will generate a transaction sending a small random number of coins to one or more customer nodes.

See below for the structure of transactions.

Each node will begin with a `hostfile.txt` listing the available nodes, in the following format:

```hostfile.txt
c1
c2
c3
m1
m2
m3
```

We use the simple notation that `c.` represents a customer, and `m.` represents a miner. 
In this example, the network has 6 nodes, with node 0 being the first customer, and node 3 being the first miner.

Valid `wallet_id` values will be one of the node ids described in `hostfile.txt`, or the special value `666` to represent the bank.
This value will be used for the `source_wallet_id` in generation of coins (to fill the initial contents of wallets, and for mining rewards), as well as for `dest_wallet_id` for destroying coins (if this is used as a test case).

## Honest customers
To begin with, we assume honest customer nodes, who only attempt to spend their coins once, and who only attempt to make transactions from their own wallet to another wallet.

This lets us begin by skipping all cryptographic signing and verification, focusing only on message, merkle tree construction, merkle tree verification.

As a possible test case, we can add some customers who attempt to double spend, and add some logic on another customer to validate the history of coins in the network.

## Transaction structure

## Proof of work
We will use the normal Bitcoin proof-of-work, but we will begin with no required `0` prefix.
This means any Nonce will succeed; to reduce thrashing in the network, we will control the rate of block production by having each miner wait for a random number of seconds between each successful hash computation.

## Messages

We will use the following message types:

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
