# lint fails for any license not in allowed
allow = ["MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "MPL-2.0", "ISC"]
deny  = ["GPL-1.0", "GPL-2.0+", "GPL-3.0+",
  "GPL-1.0-only", "GPL-1.0-or-later", "GPL-2.0-only", "GPL-2.0-or-later", "GPL-3.0-only", "GPL-3.0-or-later",
  "AGPL-1.0-only", "AGPL-1.0-or-later", "AGPL-3.0-only", "AGPL-3.0-or-later"]

override = {
  // These aren't true (yet) but they prevent us from erroring out
  "github.com/confluentinc/ccloud-sdk-go-v2/cmk" = "Apache-2.0",
  "github.com/confluentinc/ccloud-sdk-go-v2/iam" = "Apache-2.0",
  "github.com/confluentinc/ccloud-sdk-go-v2/kafkarest" = "Apache-2.0",
  "github.com/confluentinc/ccloud-sdk-go-v2/org" = "Apache-2.0",
}
