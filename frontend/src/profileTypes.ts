export type ProfileDocument = { body: string; revision: string; readOnly: boolean };

export type ProfileFilesAPI = {
  ReadDocument(id: string): Promise<ProfileDocument>;
  SaveContent(id: string, revision: string, body: string): Promise<ProfileDocument>;
};
