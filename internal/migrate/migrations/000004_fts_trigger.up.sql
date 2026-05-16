CREATE TRIGGER messages_update_search_vector_trigger
BEFORE INSERT OR UPDATE ON messages
FOR EACH ROW
EXECUTE FUNCTION messages_update_search_vector();
