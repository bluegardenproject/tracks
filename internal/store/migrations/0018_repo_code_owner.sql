ALTER TABLE repos ADD COLUMN code_owner TEXT NOT NULL DEFAULT ''; -- reviews in Review tracks narrow to its CODEOWNERS files; '' for the whole change
