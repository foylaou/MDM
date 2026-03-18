import { useState, useEffect, useMemo } from "react";
import apiClient from "../lib/apiClient";

interface Category {
  id: string;
  parent_id: string | null;
  name: string;
  level: number;
}

interface CategoryPickerProps {
  value: string | null;
  onChange: (categoryId: string | null, categoryPath: string) => void;
}

export function CategoryPicker({ value, onChange }: CategoryPickerProps) {
  const [categories, setCategories] = useState<Category[]>([]);
  const [selections, setSelections] = useState<(string | null)[]>([null, null, null]);

  useEffect(() => {
    apiClient.get("/api/categories")
      .then(({ data }) => setCategories(data.categories || []))
      .catch(() => {});
  }, []);

  // Build tree by level
  const level0 = useMemo(() => categories.filter((c) => c.level === 0), [categories]);
  const level1 = useMemo(() => {
    if (!selections[0]) return [];
    return categories.filter((c) => c.level === 1 && c.parent_id === selections[0]);
  }, [categories, selections]);
  const level2 = useMemo(() => {
    if (!selections[1]) return [];
    return categories.filter((c) => c.level === 2 && c.parent_id === selections[1]);
  }, [categories, selections]);

  // Init selections from value
  useEffect(() => {
    if (!value || categories.length === 0) return;
    const cat = categories.find((c) => c.id === value);
    if (!cat) return;

    if (cat.level === 0) {
      setSelections([cat.id, null, null]);
    } else if (cat.level === 1) {
      setSelections([cat.parent_id, cat.id, null]);
    } else if (cat.level === 2) {
      const parent = categories.find((c) => c.id === cat.parent_id);
      setSelections([parent?.parent_id || null, cat.parent_id, cat.id]);
    }
  }, [value, categories]);

  const handleChange = (level: number, id: string) => {
    const next = [...selections];
    next[level] = id || null;
    // Clear children
    for (let i = level + 1; i < 3; i++) next[i] = null;
    setSelections(next);

    // Find the deepest selected category
    const deepest = next[2] || next[1] || next[0];
    if (deepest) {
      const path = next
        .filter(Boolean)
        .map((sid) => categories.find((c) => c.id === sid)?.name)
        .filter(Boolean)
        .join(" > ");
      onChange(deepest, path);
    } else {
      onChange(null, "");
    }
  };

  return (
    <div className="flex gap-2">
      <select
        value={selections[0] || ""}
        onChange={(e) => handleChange(0, e.target.value)}
        className="select select-bordered select-sm flex-1"
      >
        <option value="">-- 品牌 --</option>
        {level0.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
      </select>

      {selections[0] && level1.length > 0 && (
        <select
          value={selections[1] || ""}
          onChange={(e) => handleChange(1, e.target.value)}
          className="select select-bordered select-sm flex-1"
        >
          <option value="">-- 類別 --</option>
          {level1.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
      )}

      {selections[1] && level2.length > 0 && (
        <select
          value={selections[2] || ""}
          onChange={(e) => handleChange(2, e.target.value)}
          className="select select-bordered select-sm flex-1"
        >
          <option value="">-- 型號 --</option>
          {level2.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
      )}
    </div>
  );
}
