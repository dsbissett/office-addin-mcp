# Save the binary path + selector so you don't retype.
$PSNativeCommandArgumentPassing = 'Standard'
$bin     = "go run ./cmd/office-addin-mcp"   # or: $bin = "office-addin-mcp"
$burl    = "http://127.0.0.1:9222"
$pattern = "taskpane"   # any unique substring of your add-in's URL

# Helper: invoke `call` with a JSON param string, pipe through jq if you have it.
function Call-Tool($tool, $paramJson) {
    & go run ./cmd/office-addin-mcp call --tool $tool --param $paramJson --browser-url $burl
}

# 1. Confirm the add-in target is visible. Adjust $pattern if nothing matches.
Call-Tool 'cdp.target.getTargets' '{}' | jq '.data.targetInfos[] | select(.type=="page") | {url,title,targetId}'

# 2. Sanity check: prove the add-in's Office.js bridge is alive.
Call-Tool 'excel.getActiveWorksheet' "{`"urlPattern`":`"$pattern`"}" | jq

# 3. Install a console capture hook in the page (idempotent). Build the JSON
#    in a double-quoted here-string so $pattern interpolates while inner JS
#    quotes stay literal.
$installJson = @"
{
  "urlPattern": "$pattern",
  "returnByValue": true,
  "expression": "(()=>{ if(window.__logs) return 'already-installed'; window.__logs=[]; ['log','info','warn','error','debug'].forEach(lvl=>{ const o=console[lvl].bind(console); console[lvl]=(...a)=>{ window.__logs.push({lvl,t:Date.now(),a:a.map(x=>{try{return JSON.parse(JSON.stringify(x))}catch(e){return String(x)}})}); o(...a); }; }); return 'installed'; })()"
}
"@
Call-Tool 'cdp.evaluate' $installJson | jq

# 4. Cause something to log. Either interact with the add-in in Excel,
#    or fire log lines synthetically via excel.runScript:
$runJson = @"
{
  "urlPattern": "$pattern",
  "script": "console.log('runScript hello', new Date().toISOString()); console.warn('warn level'); return {ok:true};"
}
"@
Call-Tool 'excel.runScript' $runJson | jq

# 5. Snapshot what's been captured so far. Run whenever you want to read.
$readJson = @"
{
  "urlPattern": "$pattern",
  "returnByValue": true,
  "expression": "JSON.stringify(window.__logs||[])"
}
"@
Call-Tool 'cdp.evaluate' $readJson | jq -r '.data.value' | jq

# 6. (Optional) Drain after reading so the next snapshot is incremental.
$drainJson = @"
{
  "urlPattern": "$pattern",
  "returnByValue": true,
  "expression": "const n=window.__logs.length;window.__logs.length=0;n"
}
"@
Call-Tool 'cdp.evaluate' $drainJson | jq
