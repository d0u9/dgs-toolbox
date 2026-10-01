package doc

const docHelp = `Start the document-tree workspace and its local web page.

Examples:
  dgs doc --root /path/to/documents
  dgs doc --port 8767

--root selects one tree instead of doc.trees; with no configured tree the
working directory is used. --port overrides the page's configured port
(default 8767). The workspace shows the page address.
Initialize a new tree with dgs doc init. Import and edit PDFs and their
sidecars through the page; verification, export, linking and explanations
are also available as the CLI actions listed below.`
