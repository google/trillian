let logPanel = document.getElementById('log');

/**
 * Logs a message to the current log div.
 * @param {string} msg the message to log.
 */
function log(msg) {
  logPanel.innerHTML += msg + '</br>';
}

/**
 * Logs a "PASS" for the specified test.
 * @param {string} t Name of test which passed.
 */
function pass(t) {
  log('<font color=\'green\'>[PASS] '+t+'</font>');
}

/**
 * Logs a "FAIL" for the specified test.
 * @param {string} t Name of test which failed.
 * @param {string} m Failure message.
 */
function fail(t, m) {
  log('<font color=\'red\'>[FAIL ]'+t+'</font>: '+m);
}

const tests = document.getElementById('tests');

/**
 * Runs tests.
 */
function test() {
  testConsistencyProof();
}

/**
 * Initialises a test in preparation for running.
 * @param {string} name The name of the test.
 * @return {string} The HTML element id to be used for drawing.
 */
function initTest(name) {
  const t = document.createElement('div');
  t.id='test_'+name;
  t.style='outline: 1px solid black; margin-bottom: 8px;';
  const h = document.createElement('h3');
  h.innerHTML=name;
  t.appendChild(h);
  const d = document.createElement('div');
  d.id=t.id+'_tree';
  d.style='margin: 4px; width: 400px';
  t.appendChild(d);
  const l = document.createElement('div');
  l.id=t.id+'_log';
  l.style='outline: 1px solid grey; background: #eee;';
  t.appendChild(l);
  logPanel = l;
  document.getElementById('tests').appendChild(t);
  return d;
}

/**
 * Tests consistency proofs.
 */
function testConsistencyProof() {
  // This proof data generate by Trillian's merkle_path.go code, and manually
  // adjusted for "float" vs. "drop" tree addressing.
  const tests = [
    {
      first: 4,
      second: 5,
      proof: [[2, 1]],
    },
    {
      first: 70,
      second: 80,
      proof: [[3, 10], [3, 11], [4, 4], [5, 3], [6, 0]],
    },
    {
      first: 31,
      second: 32,
      proof: [[0, 30], [0, 31], [1, 14], [2, 6], [3, 2], [4, 0]],
    },
    {
      first: 32,
      second: 33,
      proof: [[5, 1]],
    },
    {
      first: 65,
      second: 88,
      proof: [[1, 32], [1, 33], [2, 17], [3, 9], [4, 5], [5, 3], [6, 0]],
    },
  ];

  tests.forEach((t) => {
    const name = 'testConsistencyProof_' + t.first + '_' + t.second;
    const drawing = initTest(name);
    init(drawing, false);
    const tree = new Tree(name);
    tree.setSize(t.second);
    tree.render();
    let i = 0;
    let failed = false;
    tree.showConsistencyProof(t.first);
    tree._consistencyProof(t.first, function(n, h) {
      id=n.id().split('-');
      if (i >= t.proof.length) {
        n.stroke('#f00');
        fail(name, 'got unexpected extra proof node: '+id.toString());
        failed = true;
      } else if ((t.proof[i][0] != id[1]) || (t.proof[i][1] != id[2])) {
        n.stroke('#f00');
        fail(name, 'got '+id.toString()+', want: '+t.proof[i].toString());
        failed = true;
      }
      i++;
    });
    if (i != t.proof.length) {
      fail(name, 'got '+i+' proof nodes, want '+t.proof.length);
      failed = true;
    }
    if (!failed) {
      pass(name);
    }
  });
}
