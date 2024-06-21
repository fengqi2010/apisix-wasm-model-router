local core = require("apisix.core")
local json = require("apisix.core.json")

local plugin_name = "model-router-lua"
--https://www.cnblogs.com/lori/p/18190535

local schema = {
    type = "object",
    properties = {
        contentType = { type = "array", minItems = 1, items = { type = "string" } },
        path = { type = "string", minLength = 1 },
        field = { type = "string", minLength = 1 },
    },
    additionalProperties = false,
    require = { "contentType", "path", "field" }
}

local _M = {
    version = 0.1,
    priority = 10,
    name = plugin_name,
    schema = schema,
}

function _M.check_schema(conf)
    return core.schema.check(schema, conf)
end

function _M.access(conf, ctx)
    local headers = ngx.req.get_headers()
    local content_type = headers["Content-Type"] or headers["content-type"]
    local found = false
    for i, v in ipairs(conf.contentType) do
        if v == content_type then
            found = true
        end
    end
    if not content_type or not found then
        return
    end

    local match = ngx.re.match(ngx.var.uri, conf.path)
    if not match then
        return
    end

    ngx.req.read_body()
    local body = ngx.req.get_body_data()
    if not body then
        return
    end
    local decoded_body, err = json.decode(body)
    if not decoded_body then
        return
    end

    local field = decoded_body[conf.field]
    if field then
        core.request.set_header(ctx, "x-" .. conf.field .. "-router", field)
    end
end

function _M.header_filter(conf, ctx)
    ngx.header["x-" .. conf.field .. "-router-by"] = "lua"
end

return _M
