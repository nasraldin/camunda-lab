const BPMN_PATTERN = /\.bpmn$/i

export function bpmnFilesFromDirectoryPicker(files: FileList | File[] | null | undefined): File[] {
  if (!files) return []
  return Array.from(files).filter((file) => BPMN_PATTERN.test(file.name))
}

export function folderLabelFromFiles(files: File[]): string {
  const first = files[0]
  if (!first) return ''
  const relative = (first as File & { webkitRelativePath?: string }).webkitRelativePath
  if (relative) {
    const [root] = relative.split('/')
    return root || 'Selected folder'
  }
  return 'Selected folder'
}
