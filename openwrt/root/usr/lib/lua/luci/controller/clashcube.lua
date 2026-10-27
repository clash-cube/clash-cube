module("luci.controller.clashcube", package.seeall)

function index()
    local page = entry({"admin", "services", "clashcube"}, call("page"), _("ClashCube"), 51)
    page.dependent = false
    page.sysauth = "root"
    page.sysauth_authenticator = "htmlauth"
    entry({"admin", "services", "clashcube", "app"}, call("app")).leaf = true
    entry({"admin", "services", "clashcube", "api"}, call("api")).leaf = true
end

local fail

function page()
    require("luci.template").render("clashcube", {
        app_url = require("luci.dispatcher").build_url("admin", "services", "clashcube", "app")
    })
end

function app()
    local http = require "luci.http"
    local body = require("nixio.fs").readfile("/usr/share/clashcube/index.html")
    if not body then http.status(503, "ClashCube assets missing"); return end
    local token = require("luci.dispatcher").context.authtoken
    if not token or not token:match("^[a-fA-F0-9]+$") then return fail(403, "No authenticated token") end
    local api_url = require("luci.dispatcher").build_url("admin", "services", "clashcube", "api")
    body = body:gsub("</head>", '<meta name="clashcube-token" content="' .. token .. '"><meta name="clashcube-api" content="' .. api_url .. '/"></head>', 1)
    http.header("Cache-Control", "no-store")
    http.header("X-Frame-Options", "SAMEORIGIN")
    http.prepare_content("text/html; charset=utf-8")
    http.write(body)
end

fail = function(code, message)
    local http = require "luci.http"
    http.status(code)
    http.prepare_content("application/json")
    http.write_json({error = message})
end

-- This controller inherits LuCI's admin authentication. The per-session CSRF
-- token is embedded only in the authenticated page. Legacy uhttpd drops custom
-- request headers, so use LuCI's token query parameter rather than trusting one.
function api(...)
    local http = require "luci.http"
    if http.getenv("REQUEST_METHOD") ~= "POST" then return fail(405, "POST required") end
    if not (http.getenv("CONTENT_TYPE") or ""):match("^application/json") or
       http.formvalue("token", true) ~= require("luci.dispatcher").context.authtoken then
        return fail(403, "Valid CSRF token and JSON content required")
    end
    local origin = http.getenv("HTTP_ORIGIN")
    local host = http.getenv("HTTP_HOST")
    if origin and origin ~= "http://" .. (host or "") and origin ~= "https://" .. (host or "") then
        return fail(403, "Origin mismatch")
    end
    local parts = {...}
    local path
    if #parts == 1 and parts[1] == "events" then
        path = "/events"
    elseif #parts == 3 and parts[1] == "rpc" and
           parts[2]:match("^[A-Za-z]+$") and parts[3]:match("^[A-Za-z]+$") then
        path = "/rpc/" .. parts[2] .. "/" .. parts[3]
    else
        return fail(404, "Unknown API path")
    end
    if (tonumber(http.getenv("CONTENT_LENGTH")) or 0) > 4194304 then
        return fail(413, "Request too large")
    end
    local body = http.content() or ""
    if #body > 4194304 then return fail(413, "Request too large") end
    local sock = require("nixio").socket("unix", "stream")
    if not sock then return fail(503, "Socket unavailable") end
    sock:setsockopt("socket", "rcvtimeo", 180)
    sock:setsockopt("socket", "sndtimeo", 10)
    if not sock:connect("/var/run/clashcube/web.sock") then
        sock:close(); return fail(503, "ClashCube service is stopped")
    end
    local sent = sock:sendall("POST " .. path .. " HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\nContent-Type: application/json\r\nContent-Length: " .. #body .. "\r\n\r\n" .. body)
    if not sent then sock:close(); return fail(502, "Bridge write failed") end
    local chunks, size = {}, 0
    while true do
        local chunk = sock:recv(65536)
        if not chunk then sock:close(); return fail(502, "Bridge read failed") end
        if #chunk == 0 then break end
        size = size + #chunk
        if size > 41943040 then sock:close(); return fail(502, "Response too large") end
        chunks[#chunks + 1] = chunk
    end
    sock:close()
    local response = table.concat(chunks)
    local status = tonumber(response:match("^HTTP/1%.[01] (%d%d%d)"))
    local boundary = response:find("\r\n\r\n", 1, true)
    if not status or not boundary then return fail(502, "Invalid bridge response") end
    http.status(status)
    http.header("Cache-Control", "no-store")
    http.prepare_content("application/json")
    http.write(response:sub(boundary + 4))
end
