export function dedupCategories(categories: any[]): any[] {
  const map = new Map<number, any>()
  categories
    .filter((item: any) => item.label !== '全部')
    .forEach((item: any) => {
      const val = Number(item.value)
      if (!map.has(val)) {
        const entry: any = { label: item.label, value: val }
        if (item.children) {
          entry.children = item.children
            .filter((c: any) => c.label !== '全部')
            .map((c: any) => ({ label: c.label, value: Number(c.value) }))
        }
        map.set(val, entry)
      }
    })
  return Array.from(map.values())
}
