import React, { useState, useEffect } from 'react';
import { api, type Config } from './api';
import { X, Save, Settings } from 'lucide-react';

interface Props {
  isOpen: boolean;
  onClose: () => void;
}

export const SettingsModal: React.FC<Props> = ({ isOpen, onClose }) => {
  const [config, setConfig] = useState<Config | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setLoading(true);
      api.getConfig()
        .then((cfg: Config) => setConfig(cfg))
        .catch((err: Error) => console.error(err))
        .finally(() => setLoading(false));
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!config) return;
    
    setSaving(true);
    try {
      await api.updateConfig(config);
      onClose();
    } catch {
      alert('Failed to save configuration');
    } finally {
      setSaving(false);
    }
  };

  const handleChange = (field: string, value: string | number | boolean) => {
    if (!config) return;
    
    // Handle nested weight fields
    if (field.startsWith('weight.')) {
      const key = field.split('.')[1];
      setConfig({
        ...config,
        weights: {
          ...config.weights,
          [key]: value
        }
      });
      return;
    }
    
    setConfig({ ...config, [field]: value });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-2xl max-h-[90vh] overflow-hidden flex flex-col">
        <div className="px-6 py-4 border-b border-gray-100 flex justify-between items-center bg-gray-50/50">
          <h2 className="text-xl font-bold text-gray-900 flex items-center gap-2">
            <Settings className="w-5 h-5 text-indigo-600" />
            Cluster Configuration
          </h2>
          <button onClick={onClose} className="p-2 text-gray-400 hover:text-red-500 transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>
        
        <div className="p-6 overflow-y-auto flex-1">
          {loading || !config ? (
            <div className="text-center py-8 text-gray-500">Loading settings...</div>
          ) : (
            <form id="config-form" onSubmit={handleSubmit} className="space-y-6">
              
              {/* Hedging Section */}
              <div className="bg-indigo-50/50 rounded-xl p-5 border border-indigo-100">
                <div className="flex items-center justify-between mb-4">
                  <div>
                    <h3 className="text-sm font-bold text-indigo-900 uppercase tracking-wider">Request Hedging</h3>
                    <p className="text-xs text-indigo-600 mt-1">Duplicate delayed requests to mitigate slow nodes.</p>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input 
                      type="checkbox" 
                      className="sr-only peer" 
                      checked={config.enable_hedging}
                      onChange={(e) => handleChange('enable_hedging', e.target.checked)}
                    />
                    <div className="w-11 h-6 bg-gray-300 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-indigo-600"></div>
                  </label>
                </div>
                
                <div className="pt-2 border-t border-indigo-100/50">
                  <label className="block text-sm font-medium text-gray-700">P90 Percentile Threshold (0.0 - 1.0)</label>
                  <div className="mt-1 flex items-center gap-2">
                    <input 
                      type="number" step="0.01" min="0" max="1"
                      value={config.hedging_percentile}
                      onChange={(e) => handleChange('hedging_percentile', parseFloat(e.target.value))}
                      disabled={!config.enable_hedging}
                      className="block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm disabled:opacity-50" 
                    />
                  </div>
                </div>
              </div>

              {/* Advanced Routing Heuristics */}
              <div>
                <h3 className="text-sm font-bold text-gray-900 uppercase tracking-wider mb-4 border-b pb-2">Routing Heuristics</h3>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700">CPU Load Weight</label>
                    <input type="number" step="0.1" value={config.weights.cpu_load_weight} onChange={(e) => handleChange('weight.cpu_load_weight', parseFloat(e.target.value))} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700">Latency Weight</label>
                    <input type="number" step="0.1" value={config.weights.latency_weight} onChange={(e) => handleChange('weight.latency_weight', parseFloat(e.target.value))} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700">Success Rate Weight</label>
                    <input type="number" step="0.1" value={config.weights.success_rate_weight} onChange={(e) => handleChange('weight.success_rate_weight', parseFloat(e.target.value))} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700">Loaded Model Bonus</label>
                    <input type="number" step="0.1" value={config.weights.loaded_model_bonus} onChange={(e) => handleChange('weight.loaded_model_bonus', parseFloat(e.target.value))} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" />
                  </div>
                </div>
              </div>
              
              {/* Other Configurations */}
              <div>
                <h3 className="text-sm font-bold text-gray-900 uppercase tracking-wider mb-4 border-b pb-2">System Limits</h3>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700">Stall Timeout (Seconds)</label>
                    <input type="number" value={config.stall_timeout_sec} onChange={(e) => handleChange('stall_timeout_sec', parseInt(e.target.value))} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700">Keep Alive Duration (Seconds)</label>
                    <input type="number" value={config.keep_alive_duration_sec} onChange={(e) => handleChange('keep_alive_duration_sec', parseInt(e.target.value))} className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" />
                  </div>
                </div>
              </div>

            </form>
          )}
        </div>

        <div className="px-6 py-4 border-t border-gray-100 bg-gray-50 flex justify-end gap-3">
          <button type="button" onClick={onClose} className="px-4 py-2 bg-white text-gray-700 border border-gray-300 rounded-lg hover:bg-gray-50 text-sm font-medium transition-colors">
            Cancel
          </button>
          <button 
            type="submit" 
            form="config-form"
            disabled={saving || !config}
            className="px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 text-sm font-medium flex items-center gap-2 transition-colors focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:opacity-50"
          >
            {saving ? 'Saving...' : (
              <>
                <Save className="w-4 h-4" />
                Apply Configuration
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
