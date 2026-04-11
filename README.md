# gowa

**gowa** is a Go client library for the [WhatsApp Cloud API](https://developers.facebook.com/docs/whatsapp/cloud-api), written as a faithful Go port of the Python [pywa](https://github.com/david-lev/pywa) library.

- Full coverage of every pywa method (send messages, manage templates/flows, QR codes, calling, blocking, webhooks)
- Idiomatic Go: no decorators, no async/await — just functions, interfaces, and goroutines
- Single dependency: the entire HTTP layer is `net/http`; no third-party HTTP client required
- Webhook integration with **any** `net/http`-compatible framework (stdlib, Gin, Echo, Chi, Fiber…)
- Type-safe filter predicates with composable `And` / `Or` / `Not` combinators
- Blocking `Listen()` primitive for conversational flows inside handlers
- Full comment coverage on every exported symbol (function, param, return, error)

---

## Installation

```bash
go get github.com/tobibamidele/gowa
```

Requires **Go 1.21+** (uses generics for `Filter[T]` and `Result[T]`).

---

## Quick start — sending messages (no webhook)

```go
package main

import (
    "fmt"
    "log"

    "github.com/tobibamidele/gowa"
)

func main() {
    wa, err := gowa.New("PHONE_NUMBER_ID", "ACCESS_TOKEN")
    if err != nil {
        log.Fatal(err)
    }

    msg, err := wa.SendMessage("1234567890", "Hello from gowa! 👋")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Sent:", msg.ID)
}
```

---

## Webhook integration

### With the built-in server

```go
wa, _ := gowa.NewWithConfig(gowa.Config{
    Token:           "ACCESS_TOKEN",
    PhoneID:         "PHONE_NUMBER_ID",
    AppSecret:       "APP_SECRET",      // enables HMAC signature validation
    VerifyToken:     "my-verify-token",
    WebhookEndpoint: "/webhook",
})

wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
    fmt.Println("Got message:", msg.From.WAID)
}, gowa.FilterText)

log.Fatal(wa.ListenAndServe(":8080"))
```

### Mounted on Gin

```go
r := gin.Default()
r.Any(wa.WebhookEndpoint(), gin.WrapH(wa.Handler()))
r.Run(":8080")
```

### Mounted on Echo

```go
e := echo.New()
e.Any(wa.WebhookEndpoint(), echo.WrapHandler(wa.Handler()))
e.Start(":8080")
```

### Manual injection (any framework)

```go
// inside your own HTTP handler:
body, _ := io.ReadAll(r.Body)
sig := r.Header.Get("X-Hub-Signature-256")
status, msg := wa.HandleWebhookUpdate(body, sig)
w.WriteHeader(status)
w.Write([]byte(msg))
```

---

## Handlers

Python's `@wa.on_message(filters.text)` decorator becomes a method call in Go:

```go
// Register a message handler
wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
    msg.Reply("You said: " + *msg.Text)
}, gowa.FilterText)

// Register a callback-button handler
wa.OnCallbackButton(func(wa *gowa.WhatsApp, cb *gowa.CallbackButton) {
    fmt.Println("Button clicked:", cb.Data)
}, gowa.FilterCallbackData("confirm"))

// Register a list-selection handler
wa.OnCallbackSelection(func(wa *gowa.WhatsApp, sel *gowa.CallbackSelection) {
    fmt.Println("Selected:", sel.Data)
})

// Register a message-status handler
wa.OnMessageStatus(func(wa *gowa.WhatsApp, s *gowa.MessageStatus) {
    fmt.Printf("Message %s is now %s\n", s.ID, s.Status)
}, gowa.FilterStatusRead)

// All supported handler types:
wa.OnChatOpened(...)
wa.OnFlowCompletion(...)
wa.OnPhoneNumberChange(...)
wa.OnIdentityChange(...)
wa.OnTemplateStatusUpdate(...)
wa.OnTemplateCategoryUpdate(...)
wa.OnTemplateQualityUpdate(...)
wa.OnUserMarketingPreferences(...)
wa.OnCallConnect(...)
wa.OnCallTerminate(...)
wa.OnCallStatus(...)
wa.OnCallPermissionUpdate(...)
wa.OnRawUpdate(...)          // receives every webhook payload unfiltered
```

### Programmatic handler registration (pywa's add_handlers)

```go
spec := gowa.MessageHandlerSpec(myHandler, 10 /*priority*/, gowa.FilterText)
wa.AddHandlers(spec)
wa.RemoveHandlers(false /*silent*/, spec)
```

### Organising handlers in modules (pywa's load_handlers_modules)

```go
type BotHandlers struct{}

func (BotHandlers) Handlers() []gowa.HandlerSpec {
    return []gowa.HandlerSpec{
        gowa.MessageHandlerSpec(onText, 0, gowa.FilterText),
        gowa.CallbackButtonHandlerSpec(onButton),
    }
}

wa.LoadHandlersModules(BotHandlers{})
```

---

## Filters

Filters are typed predicates `func(*WhatsApp, T) bool`. Compose them freely:

```go
// Built-in message filters
gowa.FilterText          // msg.Type == text && msg.Text != ""
gowa.FilterImage
gowa.FilterVideo
gowa.FilterAudio
gowa.FilterVoice         // audio.Voice == true
gowa.FilterDocument
gowa.FilterSticker
gowa.FilterLocation
gowa.FilterContacts
gowa.FilterReaction
gowa.FilterReply
gowa.FilterForwarded
gowa.FilterMedia         // any attachment

// Parameterised
gowa.FilterFromWAID("1234567890")
gowa.FilterTextContains("hello")
gowa.FilterTextPrefix("/start")

// Callback button filters
gowa.FilterCallbackData("confirm")
gowa.FilterCallbackPrefix("action:")

// Status filters
gowa.FilterStatusSent
gowa.FilterStatusDelivered
gowa.FilterStatusRead
gowa.FilterStatusFailed

// Combinators
gowa.And(gowa.FilterText, gowa.FilterFromWAID("123"))
gowa.Or(gowa.FilterImage, gowa.FilterVideo)
gowa.Not(gowa.FilterForwarded)

// Custom filter
myFilter := func(wa *gowa.WhatsApp, msg *gowa.Message) bool {
    return msg.Text != nil && strings.HasPrefix(*msg.Text, "!")
}
wa.OnMessage(handler, myFilter)
```

---

## Sending messages

### Text

```go
// Simple text
wa.SendMessage("1234567890", "Hello!")

// With link preview
wa.SendMessage("1234567890", "Check this out: https://example.com",
    gowa.SendMessageOptions{PreviewURL: true})

// With quick-reply buttons
wa.SendMessage("1234567890", "Choose an option:",
    gowa.SendMessageOptions{
        Buttons: []gowa.Button{
            {ID: "yes", Title: "Yes"},
            {ID: "no",  Title: "No"},
        },
    })

// With a section list
wa.SendMessage("1234567890", "Pick a category:",
    gowa.SendMessageOptions{
        Buttons: &gowa.SectionList{
            ButtonText: "Browse",
            Sections: []gowa.Section{
                {Title: "Food", Rows: []gowa.SectionRow{
                    {ID: "pizza",  Title: "Pizza",  Description: "Cheesy goodness"},
                    {ID: "sushi",  Title: "Sushi",  Description: "Fresh rolls"},
                }},
            },
        },
    })

// Quote a previous message
wa.SendMessage("1234567890", "Got it!",
    gowa.SendMessageOptions{ReplyToMessageID: "wamid.XXX="})

// Tracker for delivery receipts
wa.SendMessage("1234567890", "Your order is ready",
    gowa.SendMessageOptions{Tracker: "order:42"})
```

### Media

```go
// From URL
wa.SendImage("1234567890", "https://example.com/photo.jpg", "Look at this!")
wa.SendVideo("1234567890", "https://example.com/video.mp4", "Cool video")
wa.SendDocument("1234567890", "https://example.com/report.pdf", "Q3 Report",
    gowa.SendMediaOptions{Filename: "Q3-Report.pdf"})
wa.SendAudio("1234567890", "https://example.com/track.mp3")
wa.SendVoice("1234567890", "https://example.com/note.ogg")
wa.SendSticker("1234567890", "https://example.com/sticker.webp")

// From local file path
wa.SendImage("1234567890", "/tmp/photo.png", "")

// From raw bytes
data, _ := os.ReadFile("image.jpg")
wa.SendImage("1234567890", data, "caption",
    gowa.SendMediaOptions{MimeType: "image/jpeg"})

// Image with buttons
wa.SendImage("1234567890", "https://example.com/product.jpg",
    "Check out this product!",
    gowa.SendMediaOptions{
        Buttons: []gowa.Button{
            {ID: "buy",   Title: "Buy Now"},
            {ID: "share", Title: "Share"},
        },
    })
```

### Location

```go
wa.SendLocation("1234567890", 37.4847, -122.1473,
    "WhatsApp HQ", "Menlo Park, 1601 Willow Rd, United States")

// Ask user to share their location
wa.RequestLocation("1234567890", "Please share your location to find nearby stores.")
```

### Contact cards

```go
wa.SendContact("1234567890", []gowa.Contact{
    {
        Name:   gowa.ContactName{FormattedName: "Jane Doe", FirstName: "Jane"},
        Phones: []gowa.ContactPhone{{Phone: "+1234567890", Type: "MOBILE"}},
        Emails: []gowa.ContactEmail{{Email: "jane@example.com", Type: "WORK"}},
    },
})
```

### Reactions

```go
wa.SendReaction("1234567890", "👍", "wamid.XXX=")
wa.RemoveReaction("1234567890", "wamid.XXX=")
```

### Templates

```go
wa.SendTemplate("1234567890", "order_confirmation", "en_US",
    gowa.SendTemplateOptions{
        Params: []map[string]any{
            {
                "type": "body",
                "parameters": []map[string]any{
                    {"type": "text", "text": "ORDER-12345"},
                },
            },
        },
    })
```

### Catalog & products

```go
// Full catalog
wa.SendCatalog("1234567890", "Browse our full catalog!", "",
    gowa.SendCatalogOptions{ThumbnailProductSKU: "SKU-001"})

// Single product
wa.SendProduct("1234567890", "catalog_id_123", "SKU-001",
    gowa.SendProductOptions{Body: "Our best seller!"})

// Product list
wa.SendProducts("1234567890", "catalog_id_123",
    "Tech Products", "Check out our latest gear!",
    []gowa.ProductsSection{
        {Title: "Phones",  SKUs: []string{"IPHONE15", "PIXEL8"}},
        {Title: "Laptops", SKUs: []string{"MBP14", "DELLXPS"}},
    })
```

---

## Reply shortcuts on updates

Every handler receives an update with built-in reply helpers:

```go
wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
    msg.Reply("Got your message!")          // quotes the message
    msg.ReplyImage("https://...", "photo")
    msg.ReplyDocument("/tmp/file.pdf", "Report")
    msg.ReplyLocation(37.4847, -122.1473, "HQ", "")
    msg.React("👍")
    msg.Unreact()
    msg.MarkAsRead()
    msg.IndicateTyping()
    msg.BlockSender()
})
```

---

## Conversational flows with Listen()

Block inside a handler until the user responds — equivalent of pywa's `wa.listen()`:

```go
wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
    if msg.Text == nil || *msg.Text != "/start" {
        return
    }

    msg.Reply("What's your name?")

    reply, err := wa.Listen(gowa.ListenOptions{
        SenderWAID: msg.From.WAID,
        Filters:    gowa.FilterText,
        Timeout:    30 * time.Second,
    })
    if err != nil {
        switch err.(type) {
        case *gowa.ListenerTimeout:
            msg.Reply("You took too long! Try /start again.")
        case *gowa.ListenerCanceled:
            msg.Reply("Cancelled.")
        }
        return
    }

    msg.Reply("Nice to meet you, " + *reply.Text + "!")
}, gowa.FilterText)
```

Or use the shortcut on the update itself:

```go
reply, err := msg.WaitForReply(wa, gowa.FilterText, 30*time.Second)
```

---

## Media management

```go
// Upload and reuse
mediaID, _ := wa.UploadMedia("https://example.com/image.jpg", "", "", "")
wa.SendImage("1234567890", mediaID, "Reused upload")

// Get a short-lived download URL
mediaURL, _ := wa.GetMediaURL("media_id_123")
fmt.Println(mediaURL.URL)     // valid for 5 minutes

// Download to disk
path, _ := wa.DownloadMedia(mediaURL.URL, "/tmp/downloads/")

// Get raw bytes
data, _ := wa.GetMediaBytes(mediaURL.URL)

// Stream (e.g. proxy to another server)
rc, _ := wa.StreamMedia(mediaURL.URL)
defer rc.Close()
io.Copy(w, rc)

// Delete
wa.DeleteMedia("media_id_123", "")
```

---

## Business profile

```go
profile, _ := wa.GetBusinessProfile("")
fmt.Println(profile.About)

about := "Open Mon–Fri 9am–5pm"
wa.UpdateBusinessProfile(gowa.UpdateBusinessProfileOptions{
    About:       &about,
    Websites:    []string{"https://example.com"},
})
```

---

## Template management

```go
// Create
wa.CreateTemplate(map[string]any{
    "name":     "order_update",
    "category": "UTILITY",
    "language": "en_US",
    "components": []map[string]any{
        {"type": "BODY", "text": "Your order {{1}} has shipped!"},
    },
})

// List with filters
templates, _ := wa.GetTemplates(gowa.GetTemplatesOptions{
    Status:   "APPROVED",
    Language: "en_US",
})
for _, t := range templates.Items {
    fmt.Println(t.Name, t.Status)
}

// Update
wa.UpdateTemplate("template_id", gowa.UpdateTemplateOptions{
    NewCategory: gowa.TemplateCategoryMarketing,
})

// Delete
wa.DeleteTemplate("order_update", "", "")

// Unpause a pacing-paused template
wa.UnpauseTemplate("template_id")

// Migrate between WABAs
wa.MigrateTemplates("source_waba_id", 0, "dest_waba_id")
```

---

## Flow management

```go
// Create a draft flow
flow, _ := wa.CreateFlow("Feedback Survey", []gowa.FlowCategory{
    gowa.FlowCategorySurvey,
})

// Upload JSON
wa.UpdateFlowJSON(flow.ID, `{"version":"5.0","screens":[...]}`)

// Publish (irreversible)
wa.PublishFlow(flow.ID)

// Deprecate when no longer needed
wa.DeprecateFlow(flow.ID)

// Handle flow data-exchange requests
wa.RegisterFlowEndpoint("/flows/survey", func(wa *gowa.WhatsApp, req *gowa.FlowRequest) (*gowa.FlowResponse, error) {
    fmt.Println("Flow action:", req.Action, "Screen:", req.Screen)
    return &gowa.FlowResponse{
        Screen: "CONFIRM",
        Data:   map[string]any{"name": req.Data["name"]},
    }, nil
})

// Get metrics
metrics, _ := wa.GetFlowMetrics(flow.ID,
    gowa.FlowMetricSent,
    gowa.FlowMetricGranularityDay)
```

---

## QR codes

```go
// Create
qr, _ := wa.CreateQRCode("Hello! How can I help?", "PNG", "")
fmt.Println(qr.QRImageURL)

// List all
codes, _ := wa.GetQRCodes("PNG", "", nil)
for _, c := range codes.Items {
    fmt.Println(c.Code, c.PrefilledMessage)
}

// Update
wa.UpdateQRCode(qr.Code, "Updated message", "")

// Delete
wa.DeleteQRCode(qr.Code)
```

---

## User blocking

```go
res, _ := wa.BlockUsers([]string{"1234567890", "0987654321"})
fmt.Printf("Blocked %d, failed %d\n", len(res.AddedUsers), len(res.FailedUsers))

wa.UnblockUsers([]string{"0987654321"})

blocked, _ := wa.GetBlockedUsers("", &gowa.Pagination{Limit: 20})
for _, u := range blocked.Items {
    fmt.Println(u.WAID)
}
```

---

## Calling

```go
// Initiate an outbound call
call, _ := wa.InitiateCall("1234567890", gowa.SessionDescription{
    Type: "offer",
    SDP:  "v=0\r\n...",
})

// Handle inbound call (inside OnCallConnect handler)
wa.OnCallConnect(func(wa *gowa.WhatsApp, c *gowa.CallConnect) {
    wa.PreAcceptCall(c.CallID, gowa.SessionDescription{Type: "answer", SDP: "..."}, "")
    wa.AcceptCall(c.CallID, gowa.SessionDescription{Type: "answer", SDP: "..."}, "", "")
    // or reject:
    wa.RejectCall(c.CallID, "")
})

// Terminate after the call
wa.TerminateCall("call_id_123", "")
```

---

## Webhook callback URL management

```go
// Register the webhook at the app level
tok, _ := wa.GetAppAccessToken("APP_ID", "APP_SECRET")
wa.SetAppCallbackURL(12345678, tok,
    "https://my-server.com/webhook", "verify-token",
    []string{"messages", "message_template_status_update"})

// Override at WABA level
wa.OverrideWABACallbackURL("https://new.example.com/webhook", "verify-token", "")
wa.DeleteWABACallbackURL("")

// Override at phone-number level
wa.OverridePhoneCallbackURL("https://new.example.com/webhook", "verify-token", "")
wa.DeletePhoneCallbackURL("")
```

---

## Error handling

All API errors are returned as `*gowa.WhatsAppError`:

```go
_, err := wa.SendMessage("1234567890", "Hello!")
if err != nil {
    var waErr *gowa.WhatsAppError
    if errors.As(err, &waErr) {
        fmt.Printf("API error %d (%s): %s\n", waErr.Code, waErr.Type, waErr.Message)
        switch gowa.Kind(waErr.Code) {
        case gowa.ErrorKindRateLimit:
            time.Sleep(60 * time.Second)
            // retry...
        case gowa.ErrorKindAuth:
            log.Fatal("Token expired or invalid")
        }
    }
}
```

---

## Configuration reference

```go
wa, err := gowa.NewWithConfig(gowa.Config{
    // Required for sending
    Token:   "EAADKQl9oJxx...",
    PhoneID: "123456789",

    // Required for template/flow/WABA management
    BusinessAccountID: "987654321",

    // Required for webhook verification challenge
    VerifyToken: "my-secret-verify-token",

    // Required for HMAC signature validation (strongly recommended)
    AppSecret: "abc123...",

    // Required for registering the callback URL at app scope
    AppID: "111222333",

    // Webhook path (default: "/webhook")
    WebhookEndpoint: "/whatsapp/webhook",

    // Graph API version (default: "22.0")
    APIVersion: "22.0",

    // Custom HTTP client (proxies, timeouts, etc.)
    HTTPClient: &http.Client{
        Timeout: 15 * time.Second,
        Transport: &http.Transport{
            Proxy: http.ProxyURL(proxyURL),
        },
    },

    // Drop updates not belonging to this PhoneID (default: true)
    FilterUpdates: true,

    // Call all matching handlers, not just the first (default: false)
    ContinueHandling: false,

    // Verify HMAC on every inbound update (default: true; requires AppSecret)
    ValidateUpdates: true,

    // RSA private key for Flow end-to-end decryption
    BusinessPrivateKey:         "-----BEGIN RSA PRIVATE KEY-----\n...",
    BusinessPrivateKeyPassword: "",
})
```

---

## Architecture

```
github.com/tobibamidele/gowa/
├── go.mod                  # module: github.com/tobibamidele/gowa
│
├── errors.go               # WhatsAppError, ErrorKind — API error hierarchy
├── types.go                # All value types: User, Message*, Media*, Button*,
│                           #   Contact, Location, Flow*, Template*, QR, Pagination…
├── update.go               # BaseUserUpdate (parent of Message, Callback*, etc.)
│                           #   + MarkAsRead / IndicateTyping / React / Reply shortcuts
├── filters.go              # Filter[T] type, And/Or/Not, all built-in predicates
├── handlers.go             # On*() registration, handler priority queue, handlerList[T]
│
├── api.go                  # graphAPI — raw net/http layer for the Meta Graph API
│                           #   All endpoints: messages, media, templates, flows,
│                           #   QR codes, blocking, calling, commerce, WABA info
│
├── webhook.go              # HTTP Handler(), ListenAndServe(), HandleWebhookUpdate()
│                           #   HMAC validation, update routing, all field dispatchers
│
├── client.go               # WhatsApp struct, New(), NewWithConfig()
│                           #   SendMessage/Image/Video/Document/Audio/Voice/Sticker
│                           #   SendReaction, SendLocation, SendContact, SendCatalog
│                           #   SendProduct, SendTemplate, MarkMessageAsRead,
│                           #   IndicateTyping, Upload/Get/Download/Stream/DeleteMedia
│                           #   GetBusinessProfile, UpdateBusinessProfile
│                           #   CreateTemplate, DeleteTemplate, GetTemplate
│                           #   CreateFlow, PublishFlow, DeleteFlow, GetFlow, GetFlows
│                           #   CreateQRCode, DeleteQRCode, BlockUsers, UnblockUsers
│                           #   InitiateCall, RegisterPhoneNumber, SetAppCallbackURL…
│
├── client_extended.go      # SendProducts, GetBusinessAccount, GetBusinessPhoneNumbers
│                           #   SetBusinessPublicKey, GetTemplates, UpdateTemplate
│                           #   CompareTemplates, MigrateTemplates, UnpauseTemplate
│                           #   UpdateFlowJSON, UpdateFlowMetadata, GetFlowAssets
│                           #   MigrateFlows, GetQRCode, GetQRCodes, UpdateQRCode
│                           #   OverrideWABACallbackURL, DeleteWABACallbackURL
│                           #   OverridePhoneCallbackURL, DeletePhoneCallbackURL
│                           #   PreAcceptCall, AcceptCall, RejectCall, TerminateCall
│                           #   GetCallPermissions, GetBlockedUsers
│                           #   UpsertAuthenticationTemplate, GetBusinessAccessToken
│
├── client_remaining.go     # StreamMedia, GetBusinessPhoneNumberSettings
│                           #   UpdateBusinessPhoneNumberSettings, GetFlowMetrics
│                           #   AddHandlers, RemoveHandlers, RemoveCallbacks
│                           #   LoadHandlersModules, HandlerSpec / HandlerModule
│
├── reply_shortcuts.go      # ReplyImage/Video/Document/Audio/Voice/Sticker/Location
│                           #   ReplyLocationRequest/Contact/Catalog/Product/Template
│                           #   WaitForReply
│
├── listeners.go            # Listen(), StopListening()
│                           #   ListenerTimeout / ListenerCanceled / ListenerStopped
│
└── util.go                 # jsonMarshal / jsonUnmarshal helpers
```

---

## pywa → gowa API mapping

| pywa | gowa |
|------|------|
| `WhatsApp(phone_id=..., token=...)` | `gowa.New(phoneID, token)` |
| `WhatsApp(server=fastapi_app, ...)` | `gowa.NewWithConfig(cfg)` + `wa.Handler()` |
| `@wa.on_message(filters.text)` | `wa.OnMessage(fn, gowa.FilterText)` |
| `@wa.on_callback_button` | `wa.OnCallbackButton(fn)` |
| `filters.text & filters.reply` | `gowa.And(gowa.FilterText, gowa.FilterReply)` |
| `filters.text \| filters.image` | `gowa.Or(gowa.FilterText, gowa.FilterImage)` |
| `~filters.forwarded` | `gowa.Not(gowa.FilterForwarded)` |
| `wa.send_message(to, text)` | `wa.SendMessage(to, text)` |
| `msg.reply("hi")` | `msg.Reply("hi")` |
| `msg.react("👍")` | `msg.React("👍")` |
| `wa.listen(to=UserUpdateListenerIdentifier(...), timeout=30)` | `wa.Listen(gowa.ListenOptions{SenderWAID: ..., Timeout: 30*time.Second})` |
| `wa.add_handlers(MessageHandler(fn, fil.text))` | `wa.AddHandlers(gowa.MessageHandlerSpec(fn, 0, gowa.FilterText))` |
| `wa.load_handlers_modules(my_module)` | `wa.LoadHandlersModules(MyHandlers{})` |
| `ListenerTimeout` | `*gowa.ListenerTimeout` |
| `WhatsAppError` | `*gowa.WhatsAppError` |

---

## License

MIT — see [LICENSE](LICENSE).
