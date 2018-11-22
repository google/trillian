const colors = {
  regularNode: '#f1f3f4',
  regularNodeStroke: '#bdc1c6',
  regularLine: '#bdc1c6',
  inclusionTarget: '#1a73e8',
  inclusionProof: '#ea4335',
  consistencyProof: '#fbbc04',
  consistencyProofOpt: '#34a853',
};

// Controls the relative layer depth of nodes at different levels.
const levelPower = 1.3;

// This is our SVG drawing area.
let draw = null;

// Whether to render the node key.
let renderKey = true;

/**
 * Initialises the SVG drawing area.
 * @param {string} element HTML element id to use for drawing.
 * @param {bool} key Whether to render the node key.
 */
function init(element, key) {
  renderKey = key;
  draw = SVG(element).size('100%', '70%');
}

/**
 * Renders the node type key.
 */
function drawKey() {
  let index = 0;
  const add = function(name, fill, stroke) {
    draw.circle(30)
        .stroke(stroke)
        .fill(fill)
        .x(10)
        .y(2+35*index);
    draw.text(name)
        .x(45)
        .y(2+8+35*index)
        .font({family: 'Roboto', size: 14});
    index++;
  };

  add('node', colors.regularNode,
      {color: colors.regularNodeStroke, width: 2});
  add('inclusion target', colors.inclusionTarget,
      {color: colors.regularNodeStroke, width: 2});
  add('inclusion proof', colors.inclusionProof,
      {color: colors.regularNodeStroke, width: 2});
  add('consistency proof', colors.regularNode,
      {color: colors.consistencyProof, width: 4});
};

/**
 * Returns the largest power of two <= n.
 * @param {number} n
 * @return {number}
 */
function largestPowerOfTwo(n) {
  if (n==0) {
    return 0;
  }
  let t = 0;
  for (let i = 0; i<32; i++) {
    const c = 1<<i;
    if (c > n) {
      return t;
    }
    t = c;
  }
  console.log('That\'s a large tree you\'ve got there, mister.', n);
  return 0;
}

/**
 * Constructs a new Tree instance.
 * @param {string} id Tree base-ID.
 */
function Tree(id) {
  this.leaves = [];
  this.layerSizes = [];
  if (!id) {
    id = '';
  }
  this.baseID=id;
}

/**
 * Applies the defaultNode stroke to n.
 * @param {object} n The node to modify.
 * @param {integer} height The node's height.
 * @return {object} The node.
 */
Tree.prototype._defaultStroke = function(n, height) {
  t = this;
  return n.stroke({
    color: colors.regularNodeStroke,
    width: new SVG.Number((height+1)/(200*t.numLevels)).to('%'),
  });
};

/**
 * Adds a new leaf to the tree.
 * @param {string} name Leaf to be added.
 */
Tree.prototype.addLeaf = function(name) {
  this.leaves.push(name);
};

let oldInclusionProofIndex = null;

/**
 * Clears the currently displayed inclusion proof, if any.
 */
Tree.prototype.clearInclusionProof = function() {
  if (oldInclusionProofIndex == null) {
    return;
  }
  const f = function(n, h) {
    n.fill(colors.regularNode);
  };
  const effective = this._dropAddressToFloatAddress(0, oldInclusionProofIndex);
  this._inclusionProof(effective.height, effective.index, f, f);
  oldInclusionProofIndex = null;
};

/**
 * Displays the inclusion proof for the given leaf index.
 * @param {number} index The index of inclusion proof to display.
 */
Tree.prototype.showInclusionProof = function(index) {
  const f1 = function(n, h) {
    n.fill(colors.inclusionTarget);
  };
  const f2 = function(n, h) {
    n.fill(colors.inclusionProof);
  };
  const effective = this._dropAddressToFloatAddress(0, index);
  this._inclusionProof(effective.height, effective.index, f1, f2);
  oldInclusionProofIndex = index;
};

/**
 * @param {number} height The height of the node.
 * @param {number} index The index of the node.
 * @return {number} A formatted node identifier for the given coordinates.
 */
Tree.prototype._nodeID = function(height, index) {
  return this.baseID + '-' + height + '-' + index;
};


/**
 * Calculates the inclusion proof for the given leaf index, and applies f2 to
 * each of the nodes present, and applies f1 to the leaf node itself.
 * @param {number} height The height of the node.
 * @param {number} index The index of the node.
 * @param {function} f1 The function applied to the node the proof is for.
 * @param {function} f2 The function applied to the proof nodes.
 */
Tree.prototype._inclusionProof = function(height, index, f1, f2) {
  f1(SVG.get(this._nodeID(height, index)), 0);
  this.pathToRoot(height, index, f2);
};

/**
 * Calculates the inclusion proof (merkle path) from the specified tree node
 * to the root.
 * @param {number} height The height of the node.
 * @param {number} index The index of the node.
 * @param {function} f The function applied to the node on the path.
 */
Tree.prototype.pathToRoot = function(height, index, f) {
  for (; height < this.numLevels; height++) {
    index ^= 1;
    f(SVG.get(this._nodeID(height, index)), height+1);
    index >>= 1;
  }
};

// Somewhere to store the size of the currently displayed consistency proof.
// Used to remove it when the uses asks for a different proof.
let oldConsistencyProof = null;

/**
 * Clears the currently displayed consistency proof, if any.
 */
Tree.prototype.clearConsistencyProof = function() {
  if (oldConsistencyProof == null) {
    return;
  }
  this._consistencyProof(oldConsistencyProof, this._defaultStroke);
  oldConsistencyProof = null;
};

/**
 * Displays the consistency proof from the specified tree size to the current
 * tree size.
 * @param {number} fromSize The size of the smaller tree to show consistency
 *                          from.
 */
Tree.prototype.showConsistencyProof = function(fromSize) {
  const w = new SVG.Number(10/this.numLevels);
  this._consistencyProof(fromSize, function(n, h) {
    n.stroke({color: colors.consistencyProof, width: w.times(h+1)});
  });
  oldConsistencyProof = fromSize;
};

/**
 * Converts a node address from "drop" (aka Trillian) to the "float" (C++)
 * addressing scheme.
 *
 * @param {number} height The node height.
 * @param {number} index The node index.
 * @return {number} The actual leaf index
 * Converts from an "effective leaf" (zero height) address to the actual node
 * address of that leaf (which, in the case of non-perfect trees may be a node
 *  whose height is greater than zero).
 */
Tree.prototype._dropAddressToFloatAddress = function(height, index) {
  // If we've been asked for an internal node, first adjust the parameters to
  // reference a leaf-level node under the requested node:
  for (let h=height; h > 0; h--) {
    index <<=1;
  }

  // Walk up the layers under we find a level which would contain the leaf.
  for (let eh=0; eh < this.layerSizes.length; eh++) {
    // Would the leaf fit at this layer?
    if (index < this.layerSizes[eh]) {
      // If we were asked for an internal node, continue walking upwards
      // the requested number of levels:
      for (;height > 0; height--) {
        eh++;
        index >>=1;
      }
      return {height: eh, index: index};
    }
    // Leaf wouldn't fit, so we'll go up again - adjust the index accordingly.
    index-=(this.layerSizes[eh]>>1);
  }
  return {height: this.layerSizes.length, index: index};
};

/**
 * Converts a leaf index from the "floating" addressing scheme to the
 * equivalent "drop" (aka Trillian) leaf index.
 *
 * @param {number} height The node height.
 * @param {number} index The node index.
 * @return {number} The "effective" leaf of a "floating leaf" node in the tree.
 * This is required because we often think about non-perfect size merkle trees
 * as having leaves which "float" upwards to be closer to the root.
 */
Tree.prototype._nodeToEffectiveLeafIndex = function(height, index) {
  if (height == 0) {
    return index;
  }
  let zeroes=height;
  let m=1<<(this.numLevels-1);
  let ei=0;
  const maxIndex = this.numLeaves-1;
  while (zeroes != 0) {
    if ((maxIndex & m) != 0) {
      ei += m;
    } else {
      zeroes--;
    }
    index &= m-1;
    m >>= 1;
  }
  ei+=index;

  return ei;
};

/**
 * Calculates the nodes which form the consistency proof from the specified
 * size to the tree's current size, and applies f to each of them.
 * @param {number} fromSize The size of the smaller tree to show consistency
 *                          from.
 * @param {function} f The function to be applied to the proof nodes.
 * @param {number} height the starting tree level of the proof.
 */
Tree.prototype._consistencyProof = function(fromSize, f) {
  if ((fromSize <= 0) || (fromSize >= this.numLeaves)) {
    return;
  }

  let index = fromSize-1;
  let height = 0;
  while ((index & 1) != 0) {
    index >>= 1;
    height += 1;
  }

  const effective = this._dropAddressToFloatAddress(height, index);
  if (index != 0) {
    f(SVG.get(this._nodeID(effective.height, effective.index)),
        effective.height+1);
  }

  this.pathToRoot(effective.height, effective.index, f);
};

/**
 * Renders a tree node.
 * @param {object} c The SVG container.
 * @param {number} index The index of the node to render.
 * @param {number} x X position of this node.
 * @param {number} height The height of the node to render.
 * @param {boolean} isLeaf Tree iff this node is considered a leaf.
 * @return {object} the SVG node.
 */
Tree.prototype._renderNode = function(c, index, x, height, isLeaf) {
  const depth = this.numLevels-height;
  const cx = x;
  const dia = new SVG.Number('25%').times(Math.pow(0.6, depth));
  const cy = new SVG.Number('99%')
      .minus(this.layerHeight.times(Math.pow(height, levelPower))
          .plus(dia.divide(2)));
  const node = this._defaultStroke(c.circle(dia), height+1)
      .cy(cy)
      .cx(cx)
      .fill(colors.regularNode)
      .id(this._nodeID(height, index));
  node.mouseover(function() {
  });
  if (isLeaf) {
    node.mouseover(function() {
      const idx = tree._nodeToEffectiveLeafIndex(height, index);
      document.getElementById('inclusionIndex').value = idx;
      updateInclusion();
    });
    node.click(function() {
      const idx = tree._nodeToEffectiveLeafIndex(height, index);
      document.getElementById('consistencyFrom').value = idx+1;
      updateConsistency();
    });
  }
  return node;
};

/**
 * Recursively renders a subtree.
 * @param {object} c The SVG container to render into.
 * @param {number} prefix The node prefix.
 * @param {number} lx Left-hand edge of space to render into.
 * @param {number} rx Right-hand edge of space to render into.
 * @param {number} n Number of leaf nodes under this node.
 * @param {number} height Height of this node in the tree.
 * @return {object} the top-most SVG node to enable tree-lines to be drawn.
 */
Tree.prototype._renderSubtree = function(c, prefix, lx, rx, n, height) {
  const xPos = lx.plus(rx.minus(lx).divide(2));
  const p = this._renderNode(c, prefix, xPos, height, n==1);
  if (n == 1) {
    return p;
  }

  let ls = largestPowerOfTwo(n);
  if (ls==n) {
    ls = n/2;
  }
  const rs = n-ls;
  const stroke = {
    color: colors.regularLine,
    width: new SVG.Number((height+1)/(400*this.numLevels)).to('%'),
  };
  const middle = lx.plus(rx.minus(lx).divide(2));
  if (ls >= 1) {
    const cPrefix = prefix<<1;
    const lp = this._renderSubtree(c, cPrefix, lx, middle, ls, height-1);
    if (lp != null) {
      c.line(p.cx(), p.cy(), lp.cx(), lp.cy()).stroke(stroke);
      c.add(lp);
    }
  }
  if (rs >= 1) {
    const cPrefix = (prefix<<1)+1;
    const rp = this._renderSubtree(c, cPrefix, middle, rx, rs, height-1);
    if (rp != null) {
      c.line(p.cx(), p.cy(), rp.cx(), rp.cy()).stroke(stroke);
      c.add(rp);
    }
  }
  c.add(p);
  return p;
};

/**
 * Renders the tree into SVG on the page.
 */
Tree.prototype.render = function() {
  this.update();

  this.layerHeight = new SVG.Number('100%')
      .divide(Math.pow(this.numLevels+1.5, levelPower+0.1)).to('%');
  this._renderSubtree(draw, 0, new SVG.Number('0%'), new SVG.Number('100%'),
      this.numLeaves, this.numLevels);
};

/**
 * Updates the tree for the new /set of leaves.
 */
Tree.prototype.update = function() {
  this.numLeaves = this.leaves.length;
  this.numLevels = Math.ceil(Math.log2(this.numLeaves));
  const maxIndex = this.numLeaves-1;
  this.layerSizes=[];
  let t=1<<(this.numLevels-1);
  for (let m = t>>1; m > 0; m >>= 1) {
    if ((maxIndex&m) == 0) {
      this.layerSizes.push(t);
      t>>=1;
      continue;
    }
    t += m;
  }
  this.layerSizes.push(t+1);
};

/**
 * Sets the tree to a given number of leaves.
 * @param {number} n Number of leaves.
 */
Tree.prototype.setSize = function(n) {
  for (let i=0; i < n; i++) {
    this.addLeaf('hi');
  }
};

/**
 * Updates the tree with a new size.
 */
function update() {
  this.tree = new Tree();
  oldConsistencyProof = null;
  oldInclusionProofIndex = null;

  this.tree.setSize(document.getElementById('treeSize').value);

  draw.clear();
  if (renderKey) {
    drawKey();
  }
  tree.render();
}

/**
 * Causes the tree to show the newly requested consistency proof.
 */
function updateConsistency() {
  tree.clearConsistencyProof();
  const cons = document.getElementById('consistencyFrom').value;
  if (cons) {
    const c = parseInt(cons);
    tree.showConsistencyProof(c);
  }
}

/**
 * Causes the tree to show the newly requested inclusion proof.
 */
function updateInclusion() {
  tree.clearInclusionProof();
  const idx = document.getElementById('inclusionIndex').value;
  if (idx) {
    const c = parseInt(idx);
    tree.showInclusionProof(c);
  }
}
