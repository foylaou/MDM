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

//go:embed migrations/007_managed_apps.up.sql
var Migration007SQL string

//go:embed migrations/008_rental_archive.up.sql
var Migration008SQL string

//go:embed migrations/009_rental_batch_fix.up.sql
var Migration009SQL string

//go:embed migrations/010_rental_checklist.up.sql
var Migration010SQL string

//go:embed migrations/011_app_icon.up.sql
var Migration011SQL string
