package db

import _ "embed"

//go:embed migrations/001_init.up.sql
var MigrationSQL string

//go:embed migrations/002_device_details.up.sql
var Migration002SQL string

//go:embed migrations/003_assets.up.sql
var Migration003SQL string

//go:embed migrations/004_rentals.up.sql
var Migration004SQL string

//go:embed migrations/005_categories.up.sql
var Migration005SQL string

//go:embed migrations/006_user_active.up.sql
var Migration006SQL string
