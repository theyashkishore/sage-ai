# backend/python/app/llm/client.py
import os
import logging
from dotenv import load_dotenv
import google.generativeai as genai
from typing import Dict, Any, List
import json
import asyncio
import time
from fastapi import HTTPException

# Load environment variables
load_dotenv()

# Configure logging
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('llm_service.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

class DatasetAnalyzer:
    """Helper class for dataset-specific analysis"""
    @staticmethod
    def infer_column_type(sample_value: str) -> str:
        """Infer the type of a column from a sample value"""
        try:
            int(sample_value)
            return "integer"
        except ValueError:
            try:
                float(sample_value)
                return "float"
            except ValueError:
                return "string"

    @staticmethod
    def identify_special_columns(schema: Dict) -> Dict[str, List[str]]:
        """Identify special column types"""
        columns = {
            "numeric": [],
            "categorical": [],
            "temporal": [],
            "textual": [],
            "identifier": []
        }
        
        for col, info in schema.items():
            col_type = info.get("inferred_type", "").lower()
            sample = info.get("sample", "")
            
            if col_type in ["integer", "float"]:
                columns["numeric"].append(col)
            elif "date" in col.lower() or "time" in col.lower():
                columns["temporal"].append(col)
            elif "id" in col.lower():
                columns["identifier"].append(col)
            elif len(sample.split()) > 3:
                columns["textual"].append(col)
            else:
                columns["categorical"].append(col)
                
        return columns

class LLMClient:
    def __init__(self):
        try:
            self._initialize_llm()
            self._initialize_rate_limiting()
            self._initialize_context_store()
            self.dataset_analyzer = DatasetAnalyzer()
            self.request_timeout = int(os.getenv("REQUEST_TIMEOUT", "90"))
            logger.info("LLM Client initialized successfully")
        except Exception as e:
            logger.error(f"Failed to initialize LLM Client: {str(e)}", exc_info=True)
            raise

    def _initialize_llm(self):
        """Initialize LLM configuration"""
        api_key = os.getenv("GEMINI_API_KEY")
        if not api_key:
            raise ValueError("GEMINI_API_KEY not found in environment variables")
            
        try:
            genai.configure(api_key=api_key)
            self.model = genai.GenerativeModel(os.getenv("GEMINI_MODEL_NAME", "gemini-1.5-flash"))
            self.generation_config = {
                'temperature': float(os.getenv("LLM_TEMPERATURE", "0.3")),
                'top_p': float(os.getenv("LLM_TOP_P", "0.8")),
                'top_k': int(os.getenv("LLM_TOP_K", "40")),
                'max_output_tokens': int(os.getenv("LLM_MAX_TOKENS", "2048")),
            }
            logger.info("LLM initialized successfully with model: %s", os.getenv("GEMINI_MODEL_NAME", "gemini-1.5-flash"))
        except Exception as e:
            logger.error("Failed to initialize LLM: %s", str(e), exc_info=True)
            raise

    def _initialize_rate_limiting(self):
        """Initialize rate limiting settings"""
        self._last_call_time = time.time()
        self._rate_limit_delay = float(os.getenv("RATE_LIMIT_DELAY", "1.0"))
        self.max_retries = int(os.getenv("LLM_MAX_RETRIES", "3"))
        logger.info("Rate limiting initialized with delay: %s seconds", self._rate_limit_delay)

    def _initialize_context_store(self):
        """Initialize context storage"""
        self._context = {}
        self._schema_metadata = {}
        logger.info("Context store initialized")

    async def _handle_rate_limit(self):
        """Handle rate limiting between requests"""
        now = time.time()
        time_since_last_call = now - self._last_call_time
        if time_since_last_call < self._rate_limit_delay:
            delay = self._rate_limit_delay - time_since_last_call
            logger.debug("Rate limiting: waiting for %.2f seconds", delay)
            await asyncio.sleep(delay)
        self._last_call_time = time.time()

    async def _generate_content(self, prompt: str) -> str:
        """Generate content with retries and error handling"""
        for attempt in range(self.max_retries):
            try:
                await self._handle_rate_limit()
                logger.debug("Generating content (attempt %d/%d)", attempt + 1, self.max_retries)
                
                response = await asyncio.wait_for(
                    asyncio.to_thread(
                        self.model.generate_content,
                        prompt,
                        generation_config=self.generation_config
                    ),
                    timeout=self.request_timeout
                )

                if not response.text:
                    raise ValueError("Empty response from LLM")
                
                logger.debug("Content generated successfully")
                return response.text
                
            except Exception as e:
                logger.error("Content generation failed (attempt %d/%d): %s", 
                           attempt + 1, self.max_retries, str(e))
                if attempt == self.max_retries - 1:
                    raise
                await asyncio.sleep(1)

    def _clean_response(self, response: str) -> str:
        """Clean LLM response text"""
        try:
            text = response.strip()
            # Remove any markdown formatting
            if '```json' in text:
                text = text.split('```json')[1].split('```')[0]
            elif '```' in text:
                text = text.split('```')[1].split('```')[0]
            return text.strip()
        except Exception as e:
            logger.error("Failed to clean response: %s", str(e))
            return response

    async def analyze_query(self, question: str, schema: Dict[str, Any]) -> Dict[str, Any]:
        """Analyze natural language query"""
        try:
            logger.info("Starting query analysis for: %s", question)
            
            # Analyze schema structure
            column_types = self.dataset_analyzer.identify_special_columns(schema)
            logger.debug("Identified column types: %s", column_types)
            
            prompt = f"""Analyze this question for the given dataset:

Question: {question}

Available Schema:
{json.dumps(schema, indent=2)}

Column Categories:
{json.dumps(column_types, indent=2)}

Return ONLY a JSON object with this structure (no additional text or explanations):
{{
    "query_type": "select",
    "required_columns": ["list", "of", "columns"],
    "conditions": ["list", "of", "conditions"],
    "sort": {{"column": "name", "order": "desc"}},
    "limit": number,
    "explanation": "brief explanation"
}}"""

            logger.debug("Analysis prompt created")
            response_text = await self._generate_content(prompt)
            logger.debug("Raw LLM response: %s", response_text)
            
            # Clean response
            cleaned_response = self._clean_response(response_text)
            logger.debug("Cleaned response: %s", cleaned_response)
            
            try:
                analysis = json.loads(cleaned_response)
                logger.info("Analysis completed successfully")
                return analysis
            except json.JSONDecodeError as e:
                logger.error("Failed to parse analysis response: %s", str(e))
                raise HTTPException(
                    status_code=500,
                    detail=f"Invalid response format: {str(e)}"
                )
                
        except Exception as e:
            logger.error("Query analysis failed: %s", str(e), exc_info=True)
            raise HTTPException(
                status_code=500,
                detail=f"Analysis failed: {str(e)}"
            )
        
    async def generate_query(self, analysis: Dict[str, Any], schema: Dict[str, Any]) -> str:
        try:
            logger.info("Starting query generation")
            
            prompt = f"""Generate a SQL query based on this analysis:

    Analysis:
    {json.dumps(analysis, indent=2)}

    Schema:
    {json.dumps(schema, indent=2)}

    Return ONLY the SQL query, no explanations or additional text."""

            response_text = await self._generate_content(prompt)
            cleaned_query = self._clean_response(response_text)
            logger.info("Query generated successfully")
            return cleaned_query  # Return just the string
                
        except Exception as e:
            logger.error("Query generation failed: %s", str(e), exc_info=True)
            raise HTTPException(
                status_code=500,
                detail=f"Query generation failed: {str(e)}"
            )

    async def validate_query(self, query: str, schema: Dict[str, Any]) -> Dict[str, Any]:
        """Validate generated SQL query"""
        try:
            logger.info("Starting query validation")
            
            prompt = f"""Validate this SQL query:

Query:
{query}

Schema:
{json.dumps(schema, indent=2)}

Return ONLY a JSON object with this structure:
{{
    "isValid": true/false,
    "issues": ["list", "of", "issues"],
    "suggestedFixes": ["list", "of", "fixes"],
    "explanation": "validation explanation"
}}"""

            response_text = await self._generate_content(prompt)
            cleaned_response = self._clean_response(response_text)
            
            try:
                validation = json.loads(cleaned_response)
                logger.info("Validation completed: isValid=%s", validation.get("isValid", False))
                return validation
            except json.JSONDecodeError as e:
                logger.error("Failed to parse validation response: %s", str(e))
                raise HTTPException(
                    status_code=500,
                    detail=f"Invalid validation response: {str(e)}"
                )
                
        except Exception as e:
            logger.error("Query validation failed: %s", str(e), exc_info=True)
            raise HTTPException(
                status_code=500,
                detail=f"Validation failed: {str(e)}"
            )

    async def heal_query(self, 
                        validation_result: Dict[str, Any],
                        original_query: str,
                        analysis: Dict[str, Any],
                        schema: Dict[str, Any]) -> Dict[str, Any]:
        """Attempt to heal an invalid query"""
        try:
            logger.info("Starting query healing process")
            logger.debug("Original query: %s", original_query)
            logger.debug("Validation issues: %s", validation_result.get("issues", []))
            
            prompt = f"""Fix this SQL query based on the validation results:

    Original Query:
    {original_query}

    Validation Issues:
    {json.dumps(validation_result, indent=2)}

    Original Analysis:
    {json.dumps(analysis, indent=2)}

    Schema:
    {json.dumps(schema, indent=2)}

    Return ONLY a JSON object with this structure:
    {{
        "healed_query": "fixed SQL query",
        "changes_made": [
            {{
                "issue": "description of what was wrong",
                "fix": "description of how it was fixed",
                "reasoning": "explanation of why this fix works"
            }}
        ],
        "requires_reanalysis": false,
        "confidence": 0.0-1.0,
        "requires_human_review": false,
        "notes": "explanation of changes"
    }}"""

            response_text = await self._generate_content(prompt)
            cleaned_response = self._clean_response(response_text)
            
            try:
                healing_result = json.loads(cleaned_response)
                logger.info("Healing completed: confidence=%s", healing_result.get("confidence", 0))
                return healing_result
            except json.JSONDecodeError as e:
                logger.error("Failed to parse healing response: %s", str(e))
                raise HTTPException(
                    status_code=500,
                    detail=f"Invalid healing response: {str(e)}"
                )
                
        except Exception as e:
            logger.error("Query healing failed: %s", str(e), exc_info=True)
            raise HTTPException(
                status_code=500,
                detail=f"Healing failed: {str(e)}"
            )

    async def process_with_healing(self,
                                question: str,
                                schema: Dict[str, Any],
                                max_healing_attempts: int = 3) -> Dict[str, Any]:
        """Process a query with automatic healing attempts"""
        try:
            healing_attempts = 0
            current_analysis = None
            current_query = None

            while healing_attempts < max_healing_attempts:
                try:
                    logger.info("Processing attempt %d/%d", healing_attempts + 1, max_healing_attempts)

                    # If we need reanalysis or this is the first attempt
                    if current_analysis is None:
                        current_analysis = await self.analyze_query(question, schema)
                        logger.info("Analysis completed")

                    # Generate SQL
                    current_query = await self.generate_query(current_analysis, schema)
                    logger.info("Query generated: %s", current_query)

                    # Validate
                    validation = await self.validate_query(current_query, schema)
                    logger.info("Validation result: isValid=%s", validation.get("isValid", False))

                    if validation.get("isValid", False):
                        return {
                            "success": True,
                            "query": current_query,
                            "analysis": current_analysis,
                            "validation": validation,
                            "healing_attempts": healing_attempts
                        }

                    # If invalid, attempt healing
                    healing_attempts += 1
                    logger.info("Starting healing attempt %d/%d", healing_attempts, max_healing_attempts)

                    healing_result = await self.heal_query(
                        validation,
                        current_query,
                        current_analysis,
                        schema
                    )

                    if healing_result.get("requires_human_review", False):
                        logger.warning("Query requires human review")
                        return {
                            "success": False,
                            "error": "Query requires human review",
                            "validation": validation,
                            "healing_attempts": healing_attempts,
                            "notes": healing_result.get("notes", "")
                        }

                    if healing_result.get("requires_reanalysis", False):
                        logger.info("Healing suggests reanalysis")
                        current_analysis = None  # Force reanalysis
                        continue

                    current_query = healing_result.get("healed_query")
                    logger.info("Applied healed query: %s", current_query)

                except Exception as e:
                    logger.error("Error in healing attempt %d: %s", healing_attempts, str(e))
                    healing_attempts += 1
                    if healing_attempts >= max_healing_attempts:
                        raise

            logger.warning("Max healing attempts reached")
            return {
                "success": False,
                "error": "Max healing attempts reached",
                "healing_attempts": healing_attempts
            }

        except Exception as e:
            logger.error("Process with healing failed: %s", str(e), exc_info=True)
            raise HTTPException(
                status_code=500,
                detail=f"Processing failed: {str(e)}"
            )

try:
    llm_client = LLMClient()
    logger.info("LLM client singleton created successfully")
except Exception as e:
    logger.error("Failed to create LLM client singleton: %s", str(e), exc_info=True)
    raise