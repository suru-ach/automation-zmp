{
  "type": "array",
  "items": {
    "type": "object",
    "required": ["channel", "email", "extraction", "invoices"],
    "properties": {
      "channel": { "enum": ["pdf", "img"] },
      "email": {
        "type": "object",
        "properties": {
          "gmail_id": { "type": "string" },
          "thread_id": { "type": "string" },
          "message_id": { "type": "string" },
          "subject": { "type": "string" },
          "date": { "type": "string", "format": "date-time" },
          "from": { "$ref": "#/$defs/address" },
          "to": { "$ref": "#/$defs/address" },
          "body": {
            "type": "object",
            "properties": {
              "text": { "type": "string" },
              "html": { "type": "string" },
              "text_as_html": { "type": "string" }
            }
          },
          "labels": { "type": "array", "items": { "type": "string" } },
          "headers": { "type": "object", "additionalProperties": { "type": "string" } }
        }
      },
      "extraction": {
        "type": "object",
        "properties": {
          "model": { "type": "string" },
          "created_at": {
            "oneOf": [
              { "type": "string", "format": "date-time" },
              { "type": "array", "items": { "type": "string", "format": "date-time" } }
            ]
          },
          "done": { "type": "boolean" },
          "done_reason": { "type": "string" },
          "response_count": { "type": "integer" }
        }
      },
      "invoices": {
        "type": "array",
        "items": { "$ref": "#/$defs/invoice" }
      }
    }
  },
  "$defs": {
    "address": {
      "type": "object",
      "properties": {
        "value": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "address": { "type": "string" },
              "name": { "type": "string" }
            }
          }
        },
        "html": { "type": "string" },
        "text": { "type": "string" }
      }
    },
    "invoice": {
      "type": "object",
      "properties": {
        "is_invoice": { "type": "boolean" },
        "reason": { "type": ["string", "null"] },
        "vendor_name": { "type": "string" },
        "invoice_number": { "type": "string" },
        "invoice_date": { "type": "string", "format": "date" },
        "po_number": { "type": "string" },
        "due_date": { "type": "string", "format": "date" },
        "currency": { "type": "string" },
        "subtotal": { "type": "string" },
        "tax_amount": { "type": "string" },
        "grand_total": { "type": "string" },
        "line_items": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "description": { "type": "string" },
              "quantity": { "type": "string" },
              "unit": { "type": "string" },
              "unit_price": { "type": "string" },
              "line_total": { "type": "string" }
            }
          }
        }
      }
    }
  }
}

