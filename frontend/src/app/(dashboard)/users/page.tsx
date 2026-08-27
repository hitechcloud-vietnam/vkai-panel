'use client';

import { useState } from 'react';
import { Users, Plus, Edit, Trash2, Shield } from 'lucide-react';

export default function UsersPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Users</h1>
          <p className="text-dark-400 mt-1">Manage users and permissions</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          Add User
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Users className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No users</h3>
          <p className="mt-2 text-dark-500">Add your first user to get started</p>
        </div>
      </div>
    </div>
  );
}
