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
<dd><ul><li>Third Party: <i>The firmware with $artifactHash is unique for {$platform, $revision} tuple</i></li><li>Third Party Builder: <i>The firmware was built from $git@tag using $tamago@tag and $usbarmory@tag and REV=... (with -trimpath)</i></li><li>Malware Scanner: <i>The firmware is functionally correct and without known attack vectors</i></li><li>WithSecure: <i>The firmware was knowingly issued by WithSecure</i></li></ul></dd>
<dt>Arbiter<sup>FIRMWARE</sup></dt>
<dd>Ecosystem</dd>
</dl>