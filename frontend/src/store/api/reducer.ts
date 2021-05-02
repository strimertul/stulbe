/* eslint-disable no-param-reassign */
import { createSlice } from '@reduxjs/toolkit';

export interface APIState {
  auth: string;
}

const initialState: APIState = {
  auth: null,
};

const apiReducer = createSlice({
  name: 'api',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    // todo
  },
});

export default apiReducer;
