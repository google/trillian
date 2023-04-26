<dl>
<dt>Claim<sup>FIRMWARE</sup></dt>
<dd><i><ol><li>The firmware with $artifactHash is unique for {$platform, $revision} tuple</li><li>The firmware was built from $git@tag using $tamago@tag and $usbarmory@tag and REV=... (with -trimpath)</li><li>The firmware is functionally correct and without known attack vectors</li><li>The firmware was knowingly issued by WithSecure</li></ol></i></dd>
<dt>Statement<sup>FIRMWARE</sup></dt>
<dd>Firmware Manifest</dd>
<dt>Claimant<sup>FIRMWARE</sup></dt>
<dd>WithSecure</dd>
<dt>Believer<sup>FIRMWARE</sup></dt>
<dd>Firmware Update Client</dd>
<dt>Verifier<sup>FIRMWARE</sup></dt>
<dd><ul><li>Third Party: <i>The firmware with $artifactHash is unique for {$platform, $revision} tuple</i></li><li>Third Party: <i>The firmware was built from $git@tag using $tamago@tag and $usbarmory@tag and REV=... (with -trimpath)</i></li><li>Malware Scanner: <i>The firmware is functionally correct and without known attack vectors</i></li><li>WithSecure: <i>The firmware was knowingly issued by WithSecure</i></li></ul></dd>
<dt>Arbiter<sup>FIRMWARE</sup></dt>
<dd>Ecosystem</dd>
</dl>
<dl>
<dt>Claim<sup>LOG_FIRMWARE</sup></dt>
<dd><i><ol><li>This data structure is append-only from any previous version</li><li>This data structure is globally consistent</li><li>This data structure contains only leaves of type `Firmware Manifest`</li></ol></i></dd>
<dt>Statement<sup>LOG_FIRMWARE</sup></dt>
<dd>Log Checkpoint</dd>
<dt>Claimant<sup>LOG_FIRMWARE</sup></dt>
<dd>WithSecure</dd>
<dt>Believer<sup>LOG_FIRMWARE</sup></dt>
<dd><ul><li>Firmware Update Client</li><li>Third Party</li><li>Malware Scanner</li><li>WithSecure</li></ul></dd>
<dt>Verifier<sup>LOG_FIRMWARE</sup></dt>
<dd><ul><li>Witness: <i>This data structure is append-only from any previous version</i></li><li>Witness Quorum: <i>This data structure is globally consistent</i></li><li>Third Party: <i>This data structure contains only leaves of type `Firmware Manifest`</i></li></ul></dd>
<dt>Arbiter<sup>LOG_FIRMWARE</sup></dt>
<dd>Ecosystem</dd>
</dl>