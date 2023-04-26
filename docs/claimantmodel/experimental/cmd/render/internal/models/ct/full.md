<dl>
<dt>Claim<sup>CERT</sup></dt>
<dd><i>I am authorized to certify $pubKey for $domain</i></dd>
<dt>Statement<sup>CERT</sup></dt>
<dd>X.509 Certificate</dd>
<dt>Claimant<sup>CERT</sup></dt>
<dd>Certificate Authority</dd>
<dt>Believer<sup>CERT</sup></dt>
<dd>Web Browser</dd>
<dt>Verifier<sup>CERT</sup></dt>
<dd>Domain Owner: <i>I am authorized to certify $pubKey for $domain</i></dd>
<dt>Arbiter<sup>CERT</sup></dt>
<dd>Browser Vendor</dd>
</dl>
<dl>
<dt>Claim<sup>LOG_CERT</sup></dt>
<dd><i><ol><li>This data structure is append-only from any previous version</li><li>This data structure is globally consistent</li><li>This data structure contains only leaves of type `X.509 Certificate`</li></ol></i></dd>
<dt>Statement<sup>LOG_CERT</sup></dt>
<dd>Log Checkpoint</dd>
<dt>Claimant<sup>LOG_CERT</sup></dt>
<dd>Log Operator</dd>
<dt>Believer<sup>LOG_CERT</sup></dt>
<dd><ul><li>Web Browser</li><li>Domain Owner</li></ul></dd>
<dt>Verifier<sup>LOG_CERT</sup></dt>
<dd><ul><li>Witness: <i>This data structure is append-only from any previous version</i></li><li>Witness Quorum: <i>This data structure is globally consistent</i></li><li>Domain Owner: <i>This data structure contains only leaves of type `X.509 Certificate`</i></li></ul></dd>
<dt>Arbiter<sup>LOG_CERT</sup></dt>
<dd>Browser Vendor</dd>
</dl>
