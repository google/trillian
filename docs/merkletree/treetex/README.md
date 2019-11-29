Treetex
=======

`treetex` is a tool for drawing/visualising Merkle trees.

Ensure you have latex installed on your system if you want to this; the tool
outputs `LaTeX` commands to stdout, and so needs to be used in conjuction with
that!

For debian-derived systems, `sudo apt install texlive` should do the trick.


Usage
-----

```bash
go run github.com/google/trillian/docs/merkletree/treetex | latex
```
(if you've not used that much tex - that'll create a file called `texput.pdf`)
You can use `okular` (or your favourite PDF viewer) to view it.

Use `--tree_size` to, er, set the tree size.


Features
--------
Treetex has a few tricks up its sleeveseses:

### Ephemeral nodes

This tool draws trees with all the leaves at the same level as opposed to floating them up to their real level - mainly because this is how the trees in RFC6962 are drawn.
Nodes which aren't yet final are displayed with no border.

### Large trees
When drawing 'large' tree sizes, set the --megamode_threshold (default is 4) to "collapse" perfect subtrees with at least this many levels.

```bash
go run github.com/google/trillian/docs/merkletree/treetex --tree_size=100023 --megamode_threshold=4
```
![large tree](images/large.png)


### Inclusion proofs
highlight inclusion proofs

```bash
go run github.com/google/trillian/docs/merkletree/treetex --tree_size=23 --inclusion=13
```

![inclusion proof](images/inclusion.png)

### Explicit leaf data
Choose-your-own-~~adventure~~-data.
Takes LaTex commands too.

```bash
go run github.com/google/trillian/docs/merkletree/treetex --leaf_data='entry1,thing too,${\alpha}$,${\frac{bananas_{\beta}}{\delta}}$,{\LaTeX}'
```

![leaf data](images/leafdata.png)

### Explain-Merkle-trees-mode
Draw diagrams which explain what each node hash is.

```bash
go run github.com/google/trillian/docs/merkletree/treetex --tree_size=7 --node_format=hash
```

![hash mode](images/hashmode.png)

### Compact Range proofs
Highlight node which make up (multiple) range proofs

```bash
go run github.com/google/trillian/docs/merkletree/treetex --tree_size=18 --ranges=1:3,7:13
```

![compact ranges](images/compactrange.png)

