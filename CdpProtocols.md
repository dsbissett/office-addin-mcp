# Required Tools:
---
Target.createTarget
Target.getTargets
Target.attachToTarget
Target.activateTarget
Target.closeTarget

Page.enable
Page.navigate
Page.reload
Page.stopLoading
Page.bringToFront
Page.captureScreenshot
Page.printToPDF
Page.handleJavaScriptDialog
Page.addScriptToEvaluateOnNewDocument
Page.createIsolatedWorld

Runtime.enable
Runtime.evaluate
Runtime.callFunctionOn
Runtime.awaitPromise
Runtime.addBinding
Runtime.releaseObject
Runtime.releaseObjectGroup

DOM.enable
DOM.getDocument
DOM.querySelector
DOM.querySelectorAll
DOM.resolveNode
DOM.focus
DOM.scrollIntoViewIfNeeded
DOM.setFileInputFiles

Input.dispatchKeyEvent
Input.dispatchMouseEvent
Input.dispatchTouchEvent
Input.insertText
Input.dispatchDragEvent
Input.synthesizeScrollGesture
Input.synthesizeTapGesture

Network.enable
Network.setExtraHTTPHeaders
Network.setCookie
Network.setCookies
Network.getCookies
Network.clearBrowserCache
Network.clearBrowserCookies
Network.setCacheDisabled
Network.setBypassServiceWorker
Network.setBlockedURLs

Fetch.enable
Fetch.continueRequest
Fetch.fulfillRequest
Fetch.failRequest
Fetch.continueWithAuth

Emulation.setDeviceMetricsOverride
Emulation.setUserAgentOverride
Emulation.setTimezoneOverride
Emulation.setGeolocationOverride
Emulation.setTouchEmulationEnabled
Emulation.setCPUThrottlingRate
Emulation.setScriptExecutionDisabled

Browser.setWindowBounds
Browser.getWindowForTarget
Browser.setDownloadBehavior
Browser.setPermission
Browser.resetPermissions

---

# Core browser / tab / target control
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/Browser/

Browser.close
Browser.getVersion
Browser.resetPermissions
Browser.cancelDownload
Browser.crash
Browser.crashGpuProcess
Browser.executeBrowserCommand
Browser.getBrowserCommandLine
Browser.getWindowBounds
Browser.getWindowForTarget
Browser.setContentsSize
Browser.setDockTile
Browser.setDownloadBehavior
Browser.setPermission
Browser.setWindowBounds
Browser.grantPermissions              # 

---

## Target
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/Target/

Target.activateTarget
Target.attachToTarget
Target.closeTarget
Target.createBrowserContext
Target.createTarget
Target.detachFromTarget
Target.disposeBrowserContext
Target.getBrowserContexts
Target.getTargets
Target.setAutoAttach
Target.setDiscoverTargets
Target.sendMessageToTarget            # 
Target.attachToBrowserTarget
Target.autoAttachRelated
Target.exposeDevToolsProtocol
Target.getDevToolsTarget
Target.getTargetInfo
Target.openDevTools
Target.setRemoteLocations

---

# Page navigation, lifecycle, screenshots, documents, dialogs
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/Page/

Page.addScriptToEvaluateOnNewDocument
Page.bringToFront
Page.captureScreenshot
Page.close
Page.createIsolatedWorld
Page.disable
Page.enable
Page.getAppManifest
Page.getFrameTree
Page.getLayoutMetrics
Page.getNavigationHistory
Page.handleJavaScriptDialog
Page.navigate
Page.navigateToHistoryEntry
Page.printToPDF
Page.reload
Page.removeScriptToEvaluateOnNewDocument
Page.resetNavigationHistory
Page.setBypassCSP
Page.setDocumentContent
Page.setInterceptFileChooserDialog
Page.setLifecycleEventsEnabled
Page.stopLoading
Page.clearGeolocationOverride          # 
Page.setGeolocationOverride            # 
Page.addCompilationCache
Page.captureSnapshot
Page.clearCompilationCache
Page.crash
Page.generateTestReport
Page.getAdScriptAncestry
Page.getAnnotatedPageContent
Page.getAppId
Page.getInstallabilityErrors
Page.getOriginTrials
Page.getPermissionsPolicyState
Page.getResourceContent
Page.getResourceTree
Page.produceCompilationCache
Page.screencastFrameAck
Page.searchInResource
Page.setAdBlockingEnabled
Page.setFontFamilies
Page.setFontSizes
Page.setPrerenderingAllowed
Page.setRPHRegistrationMode
Page.setSPCTransactionMode
Page.setWebLifecycleState
Page.startScreencast
Page.stopScreencast
Page.waitForDebugger
Page.addScriptToEvaluateOnLoad          # 
Page.clearDeviceMetricsOverride         # 
Page.clearDeviceOrientationOverride     # 
Page.deleteCookie                       # 
Page.getManifestIcons                   # 
Page.removeScriptToEvaluateOnLoad       # 
Page.setDeviceMetricsOverride           # 
Page.setDeviceOrientationOverride       # 
Page.setDownloadBehavior                # 
Page.setTouchEmulationEnabled           # 

---

# User Input / Browser Interaction
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/Input/

Input.cancelDragging
Input.dispatchKeyEvent
Input.dispatchMouseEvent
Input.dispatchTouchEvent
Input.setIgnoreInputEvents
Input.dispatchDragEvent
Input.emulateTouchFromMouseEvent
Input.imeSetComposition
Input.insertText
Input.setInterceptDrags
Input.synthesizePinchGesture
Input.synthesizeScrollGesture
Input.synthesizeTapGesture

---

# JavaScript execution / runtime object control
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/Runtime/

Runtime.addBinding
Runtime.awaitPromise
Runtime.callFunctionOn
Runtime.compileScript
Runtime.disable
Runtime.discardConsoleEntries
Runtime.enable
Runtime.evaluate
Runtime.getProperties
Runtime.globalLexicalScopeNames
Runtime.queryObjects
Runtime.releaseObject
Runtime.releaseObjectGroup
Runtime.removeBinding
Runtime.runIfWaitingForDebugger
Runtime.runScript
Runtime.setAsyncCallStackDepth
Runtime.getExceptionDetails
Runtime.getHeapUsage
Runtime.getIsolateId
Runtime.setCustomObjectFormatterEnabled
Runtime.setMaxCallStackSizeToCapture
Runtime.terminateExecution

--

# Debugger control / live script editing
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/Debugger/

Debugger.continueToLocation
Debugger.disable
Debugger.enable
Debugger.evaluateOnCallFrame
Debugger.getPossibleBreakpoints
Debugger.getScriptSource
Debugger.pause
Debugger.removeBreakpoint
Debugger.restartFrame
Debugger.resume
Debugger.searchInContent
Debugger.setAsyncCallStackDepth
Debugger.setBreakpoint
Debugger.setBreakpointByUrl
Debugger.setBreakpointsActive
Debugger.setInstrumentationBreakpoint
Debugger.setPauseOnExceptions
Debugger.setScriptSource
Debugger.setSkipAllPauses
Debugger.setVariableValue
Debugger.stepInto
Debugger.stepOut
Debugger.stepOver
Debugger.getWasmBytecode                # 
Debugger.disassembleWasmModule
Debugger.getStackTrace
Debugger.nextWasmDisassemblyChunk
Debugger.setBlackboxedRanges
Debugger.setBlackboxExecutionContexts
Debugger.setBlackboxPatterns
Debugger.setBreakpointOnFunctionCall
Debugger.setReturnValue
Debugger.pauseOnAsyncCall               # 

---

# DOM inspection and mutation
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/DOM/

DOM.describeNode
DOM.disable
DOM.enable
DOM.focus
DOM.getAttributes
DOM.getBoxModel
DOM.getDocument
DOM.getNodeForLocation
DOM.getOuterHTML
DOM.hideHighlight
DOM.highlightNode
DOM.highlightRect
DOM.moveTo
DOM.querySelector
DOM.querySelectorAll
DOM.removeAttribute
DOM.removeNode
DOM.requestChildNodes
DOM.requestNode
DOM.resolveNode
DOM.scrollIntoViewIfNeeded
DOM.setAttributesAsText
DOM.setAttributeValue
DOM.setFileInputFiles
DOM.setNodeName
DOM.setNodeValue
DOM.setOuterHTML
DOM.getFlattenedDocument                # 
DOM.collectClassNamesFromSubtree
DOM.copyTo
DOM.discardSearchResults
DOM.forceShowPopover
DOM.getAnchorElement
DOM.getContainerForNode
DOM.getContentQuads
DOM.getDetachedDomNodes
DOM.getElementByRelation
DOM.getFileInfo
DOM.getFrameOwner
DOM.getNodesForSubtreeByStyle
DOM.getNodeStackTraces
DOM.getQueryingDescendantsForContainer
DOM.getRelayoutBoundary
DOM.getSearchResults
DOM.getTopLayerElements
DOM.markUndoableState
DOM.performSearch
DOM.pushNodeByPathToFrontend
DOM.pushNodesByBackendIdsToFrontend
DOM.redo
DOM.setInspectedNode
DOM.setNodeStackTracesEnabled
DOM.undo

---

# CSS / visual state mutation
---
> https://chromedevtools.github.io/devtools-protocol/tot/CSS/

CSS.addRule
CSS.collectClassNames
CSS.createStyleSheet
CSS.disable
CSS.enable
CSS.forcePseudoState
CSS.forceStartingStyle
CSS.getBackgroundColors
CSS.getComputedStyleForNode
CSS.getInlineStylesForNode
CSS.getMatchedStylesForNode
CSS.getMediaQueries
CSS.getPlatformFontsForNode
CSS.getStyleSheetText
CSS.setEffectivePropertyValueForNode
CSS.setKeyframeKey
CSS.setMediaText
CSS.setPropertyRulePropertyName
CSS.setRuleSelector
CSS.setStyleSheetText
CSS.setStyleTexts
CSS.startRuleUsageTracking
CSS.stopRuleUsageTracking
CSS.takeCoverageDelta
CSS.getAnimatedStylesForNode
CSS.getEnvironmentVariables
CSS.getLayersForNode
CSS.getLocationForSelector
CSS.getLonghandProperties
CSS.resolveValues
CSS.setContainerQueryText
CSS.setLocalFontsEnabled
CSS.setNavigationText
CSS.setScopeText
CSS.setSupportsText
CSS.takeComputedStyleUpdates
CSS.trackComputedStyleUpdates
CSS.trackComputedStyleUpdatesForNode

---

# Environment / device / viewport emulation
---
> docs: https://chromedevtools.github.io/devtools-protocol/tot/Emulation/

Emulation.clearDeviceMetricsOverride
Emulation.clearGeolocationOverride
Emulation.clearIdleOverride
Emulation.setCPUThrottlingRate
Emulation.setDefaultBackgroundColorOverride
Emulation.setDeviceMetricsOverride
Emulation.setEmulatedMedia
Emulation.setEmulatedOSTextScale
Emulation.setEmulatedVisionDeficiency
Emulation.setGeolocationOverride
Emulation.setIdleOverride
Emulation.setScriptExecutionDisabled
Emulation.setTimezoneOverride
Emulation.setTouchEmulationEnabled
Emulation.setUserAgentOverride
Emulation.canEmulate                       # 
Emulation.addScreen
Emulation.clearDevicePostureOverride
Emulation.clearDisplayFeaturesOverride
Emulation.getOverriddenSensorInformation
Emulation.getScreenInfos
Emulation.removeScreen
Emulation.resetPageScaleFactor
Emulation.setAutoDarkModeOverride
Emulation.setAutomationOverride
Emulation.setDataSaverOverride
Emulation.setDevicePostureOverride
Emulation.setDisabledImageTypes
Emulation.setDisplayFeaturesOverride
Emulation.setDocumentCookieDisabled
Emulation.setEmitTouchEventsForMouse
Emulation.setFocusEmulationEnabled
Emulation.setHardwareConcurrencyOverride
Emulation.setLocaleOverride
Emulation.setPageScaleFactor
Emulation.setPressureDataOverride
Emulation.setPressureSourceOverrideEnabled
Emulation.setPressureStateOverride
Emulation.setPrimaryScreen
Emulation.setSafeAreaInsetsOverride
Emulation.setScrollbarsHidden
Emulation.setSensorOverrideEnabled
Emulation.setSensorOverrideReadings
Emulation.setSmallViewportHeightDifferenceOverride
Emulation.setVirtualTimePolicy
Emulation.updateScreen
Emulation.setNavigatorOverrides             # 
Emulation.setVisibleSize                    # 

---

# Network control / request and response manipulation
---
> https://chromedevtools.github.io/devtools-protocol/tot/Network/

Network.clearBrowserCache
Network.clearBrowserCookies
Network.deleteCookies
Network.disable
Network.enable
Network.getCookies
Network.getRequestPostData
Network.getResponseBody
Network.setBypassServiceWorker
Network.setCacheDisabled
Network.setCookie
Network.setCookies
Network.setExtraHTTPHeaders
Network.setUserAgentOverride
Network.canClearBrowserCache             # 
Network.canClearBrowserCookies           # 
Network.canEmulateNetworkConditions      # 
Network.emulateNetworkConditions         # 
Network.getAllCookies                    # 
Network.clearAcceptedEncodingsOverride
Network.configureDurableMessages
Network.deleteDeviceBoundSession
Network.emulateNetworkConditionsByRule
Network.enableDeviceBoundSessions
Network.enableReportingApi
Network.fetchSchemefulSite
Network.getCertificate
Network.getResponseBodyForInterception
Network.getSecurityIsolationStatus
Network.loadNetworkResource
Network.overrideNetworkState
Network.replayXHR
Network.searchInResponseBody
Network.setAcceptedEncodings
Network.setAttachDebugStack
Network.setBlockedURLs
Network.setCookieControls
Network.streamResourceContent
Network.takeResponseBodyForInterceptionAsStream
Network.continueInterceptedRequest       # 
Network.setRequestInterception           # 

---

# Fetch.*
---
> https://chromedevtools.github.io/devtools-protocol/tot/Fetch/

Fetch.continueRequest
Fetch.continueWithAuth
Fetch.disable
Fetch.enable
Fetch.failRequest
Fetch.fulfillRequest
Fetch.getResponseBody
Fetch.takeResponseBodyAsStream
Fetch.continueResponse

---

# Storage, cookies, origin data
---
> https://chromedevtools.github.io/devtools-protocol/tot/Storage/

Storage.clearCookies
Storage.clearDataForOrigin
Storage.clearDataForStorageKey
Storage.getCookies
Storage.getUsageAndQuota
Storage.setCookies
Storage.setProtectedAudienceKAnonymity
Storage.trackCacheStorageForOrigin
Storage.trackCacheStorageForStorageKey
Storage.trackIndexedDBForOrigin
Storage.trackIndexedDBForStorageKey
Storage.untrackCacheStorageForOrigin
Storage.untrackCacheStorageForStorageKey
Storage.untrackIndexedDBForOrigin
Storage.untrackIndexedDBForStorageKey
Storage.getStorageKeyForFrame             # 
Storage.clearSharedStorageEntries
Storage.clearTrustTokens
Storage.deleteSharedStorageEntry
Storage.deleteStorageBucket
Storage.getInterestGroupDetails
Storage.getRelatedWebsiteSets
Storage.getSharedStorageEntries
Storage.getSharedStorageMetadata
Storage.getStorageKey
Storage.getTrustTokens
Storage.overrideQuotaForOrigin
Storage.resetSharedStorageBudget
Storage.runBounceTrackingMitigations
Storage.setInterestGroupAuctionTracking
Storage.setInterestGroupTracking
Storage.setSharedStorageEntry
Storage.setSharedStorageTracking
Storage.setStorageBucketTracking

---

# WebAuthn / virtual authenticator control
---
> https://chromedevtools.github.io/devtools-protocol/tot/WebAuthn/

WebAuthn.addCredential
WebAuthn.addVirtualAuthenticator
WebAuthn.clearCredentials
WebAuthn.disable
WebAuthn.enable
WebAuthn.getCredential
WebAuthn.getCredentials
WebAuthn.removeCredential
WebAuthn.removeVirtualAuthenticator
WebAuthn.setAutomaticPresenceSimulation
WebAuthn.setCredentialProperties
WebAuthn.setResponseOverrideBits
WebAuthn.setUserVerified

# Accessiblity
---
> https://chromedevtools.github.io/devtools-protocol/tot/Accessibility/

Accessibility.disable
Accessibility.enable
Accessibility.getAXNodeAndAncestors 
Accessibility.getChildAXNodes 
Accessibility.getFullAXTree 
Accessibility.getPartialAXTree 
Accessibility.getRootAXNode 
Accessibility.queryAXTree 

# Animation
---
> https://chromedevtools.github.io/devtools-protocol/tot/Animation/
Animation.disable
Animation.enable
Animation.getCurrentTime
Animation.getPlaybackRate
Animation.releaseAnimations
Animation.resolveAnimation
Animation.seekAnimations
Animation.setPaused
Animation.setPlaybackRate
Animation.setTiming

# Background Service Domain
---
> https://chromedevtools.github.io/devtools-protocol/tot/BackgroundService/

BackgroundService.clearEvents
BackgroundService.setRecording
BackgroundService.startObserving
BackgroundService.stopObserving

# Cache Storage
> https://chromedevtools.github.io/devtools-protocol/tot/CacheStorage/
CacheStorage.deleteCache
CacheStorage.deleteEntry
CacheStorage.requestCachedResponse
CacheStorage.requestCacheNames
CacheStorage.requestEntries

# DOMDebugger
> https://chromedevtools.github.io/devtools-protocol/tot/DOMDebugger/
DOMDebugger.getEventListeners
DOMDebugger.removeDOMBreakpoint
DOMDebugger.removeEventListenerBreakpoint
DOMDebugger.removeXHRBreakpoint
DOMDebugger.setDOMBreakpoint
DOMDebugger.setEventListenerBreakpoint
DOMDebugger.setXHRBreakpoint
DOMDebugger.setBreakOnCSPViolation 
DOMDebugger.removeInstrumentationBreakpoint 
DOMDebugger.setInstrumentationBreakpoint 

EOF